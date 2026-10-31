package api

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"mace/internal/records"
)

// Nodes are the machines in the owner's infrastructure (docs/DESIGN-HUB-WORKER.md).
// A machine joins once with a short-lived code; after that, machines talk
// with requests signed by their node keys. Tailscale carries the traffic and
// proves the machines belong to the owner.

// Node is a joined machine as every other machine stores it.
type Node struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	PublicKey    []byte    `json:"public_key"`
	URL          string    `json:"url"`
	JoinedAt     time.Time `json:"joined_at"`
	IntroducedBy string    `json:"introduced_by,omitempty"`
}

// NodeView adds what this machine currently knows about a node.
type NodeView struct {
	Node
	Self      bool       `json:"self,omitempty"`
	Available bool       `json:"available"`
	LastSeen  *time.Time `json:"last_seen,omitempty"`
	Problem   string     `json:"problem,omitempty"`
	// Checking means this machine has not tried to reach the node yet.
	Checking bool `json:"checking,omitempty"`
}

type joinInvite struct {
	Code    string    `json:"code"`
	URL     string    `json:"url"`
	Expires time.Time `json:"expires"`
}

type nodePong struct {
	Node  Node   `json:"node"`
	Nodes []Node `json:"nodes"`
}

type nodeState struct {
	mu   sync.Mutex
	seen map[string]time.Time
	fail map[string]string
	// asked is when the check behind each node's current state started, so a
	// slow, older check never overwrites a newer result.
	asked map[string]time.Time
}

const (
	inviteLifetime = 10 * time.Minute
	signatureSkew  = 2 * time.Minute
	nodeCheckEvery = 15 * time.Second
)

func (n Node) valid() bool {
	return len(n.PublicKey) == ed25519.PublicKeySize && records.NodeID(n.PublicKey) == n.ID && strings.HasPrefix(n.URL, "http")
}

// selfNode describes this machine to others.
func (h *APIHandler) selfNode() Node {
	if h.records == nil {
		return Node{}
	}
	id := h.records.Identity()
	name := os.Getenv("MACE_NODE_NAME")
	if name == "" {
		name = h.nodeInfo.HostName
	}
	return Node{ID: id.ID, Name: name, PublicKey: id.Public, URL: h.selfURL()}
}

var tailnetIP = struct {
	sync.Mutex
	at time.Time
	ip string
}{}

// selfURL is where other machines reach this one: MACE_PUBLIC_URL if set,
// otherwise this machine's Tailscale address.
func (h *APIHandler) selfURL() string {
	if h.advertise != "" {
		return h.advertise
	}
	if v := os.Getenv("MACE_PUBLIC_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	tailnetIP.Lock()
	defer tailnetIP.Unlock()
	if time.Since(tailnetIP.at) > time.Minute {
		tailnetIP.at, tailnetIP.ip = time.Now(), ""
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "tailscale", "ip", "-4").Output(); err == nil {
			tailnetIP.ip = strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
		}
	}
	if tailnetIP.ip == "" || h.port == 0 {
		return ""
	}
	return "http://" + tailnetIP.ip + ":" + strconv.Itoa(h.port)
}

func (h *APIHandler) knownNodes(ctx context.Context) []Node {
	raw, _ := h.store.Documents(ctx, "node")
	out := []Node{}
	for _, b := range raw {
		var n Node
		if json.Unmarshal(b, &n) == nil && n.valid() {
			out = append(out, n)
		}
	}
	return out
}

// learnNodes stores nodes this machine did not know, introduced by another
// node. Removed nodes and this machine itself are skipped. It returns the
// nodes that were new.
func (h *APIHandler) learnNodes(ctx context.Context, nodes []Node, introducer string) []Node {
	self := h.selfNode().ID
	added := []Node{}
	for _, n := range nodes {
		if !n.valid() || n.ID == self {
			continue
		}
		var existing Node
		if h.store.Document(ctx, "node", n.ID, &existing) == nil {
			if existing.URL != n.URL && introducer == n.ID {
				existing.URL = n.URL
				h.store.PutDocument(ctx, "node", n.ID, existing)
			}
			continue
		}
		var gone struct{}
		if h.store.Document(ctx, "node-removed", n.ID, &gone) == nil {
			continue
		}
		if n.JoinedAt.IsZero() {
			n.JoinedAt = time.Now().UTC()
		}
		n.IntroducedBy = introducer
		h.store.PutDocument(ctx, "node", n.ID, n)
		h.audit("mace", "NODE_JOINED", n.ID, map[string]any{"name": n.Name, "introduced_by": introducer})
		added = append(added, n)
		// Check a new machine now rather than at the next round.
		go h.checkNode(context.Background(), n, 0)
	}
	return added
}

// markNode records the result of contact with a node that began at asked.
func (h *APIHandler) markNode(id, problem string, asked time.Time) {
	h.nodeHealth.mu.Lock()
	defer h.nodeHealth.mu.Unlock()
	if h.nodeHealth.seen == nil {
		h.nodeHealth.seen, h.nodeHealth.fail, h.nodeHealth.asked = map[string]time.Time{}, map[string]string{}, map[string]time.Time{}
	}
	if asked.Before(h.nodeHealth.asked[id]) {
		return
	}
	h.nodeHealth.asked[id] = asked
	if problem == "" {
		h.nodeHealth.seen[id] = time.Now()
		delete(h.nodeHealth.fail, id)
	} else {
		h.nodeHealth.fail[id] = problem
	}
}

// listNodes serves GET /api/v1/nodes: this machine first, then the others.
func (h *APIHandler) listNodes(w http.ResponseWriter, r *http.Request) {
	self := h.selfNode()
	if self.ID == "" {
		http.Error(w, "This machine has no node identity; see the log.", 503)
		return
	}
	now := time.Now()
	out := []NodeView{{Node: self, Self: true, Available: true, LastSeen: &now}}
	h.nodeHealth.mu.Lock()
	for _, n := range h.knownNodes(r.Context()) {
		v := NodeView{Node: n, Problem: h.nodeHealth.fail[n.ID]}
		_, tried := h.nodeHealth.fail[n.ID]
		_, ok := h.nodeHealth.seen[n.ID]
		v.Checking = !tried && !ok
		if seen, ok := h.nodeHealth.seen[n.ID]; ok {
			v.LastSeen = &seen
			v.Available = v.Problem == "" && time.Since(seen) < 3*nodeCheckEvery
		}
		out = append(out, v)
	}
	h.nodeHealth.mu.Unlock()
	writeJSON(w, 200, out)
}

// nodeIdentity serves GET /api/v1/node/identity. Public keys are not secret.
func (h *APIHandler) nodeIdentity(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, h.selfNode())
}

func normalizeCode(code string) string {
	return strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' {
			return -1
		}
		if r >= 'a' && r <= 'z' {
			return r - 32
		}
		return r
	}, code)
}

// invite serves POST /api/v1/nodes/invite: a one-time code another machine
// uses to join within ten minutes.
func (h *APIHandler) invite(w http.ResponseWriter, r *http.Request) {
	self := h.selfNode()
	if self.URL == "" {
		http.Error(w, "Tailscale is not running on this machine, so other machines cannot reach it. Run sudo tailscale up, or set MACE_PUBLIC_URL.", 409)
		return
	}
	b := make([]byte, 10)
	rand.Read(b)
	raw := base32.StdEncoding.EncodeToString(b)
	code := raw[0:4] + "-" + raw[4:8] + "-" + raw[8:12] + "-" + raw[12:16]
	inv := joinInvite{Code: code, URL: self.URL, Expires: time.Now().Add(inviteLifetime).UTC()}
	if err := h.store.PutDocument(r.Context(), "join-invite", hashHex(normalizeCode(code)), map[string]time.Time{"expires": inv.Expires}); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, inv)
}

func joinProof(code, node string) []byte {
	return []byte("mace-join/1\n" + normalizeCode(code) + "\n" + node)
}

// join serves POST /api/v1/nodes/join on the new machine: it asks the machine
// at url to accept it with the code printed there.
func (h *APIHandler) join(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL  string `json:"url"`
		Code string `json:"code"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil || req.URL == "" || req.Code == "" {
		http.Error(w, "Send the address and join code printed by mace join --invite.", 400)
		return
	}
	self := h.selfNode()
	if self.URL == "" {
		http.Error(w, "Tailscale is not running on this machine, so the other machine cannot reach it. Run sudo tailscale up, or set MACE_PUBLIC_URL.", 409)
		return
	}
	body, _ := json.Marshal(map[string]any{"code": req.Code, "node": self, "proof": h.records.Identity().Sign(joinProof(req.Code, self.ID))})
	url := strings.TrimRight(req.URL, "/")
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	call, _ := http.NewRequestWithContext(ctx, "POST", url+"/api/v1/nodes/accept", bytes.NewReader(body))
	call.Header.Set("Content-Type", "application/json")
	res, err := (&http.Client{Transport: peerTransport}).Do(call)
	if err != nil {
		http.Error(w, "Could not reach "+url+": "+err.Error(), 502)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 1000))
		http.Error(w, strings.TrimSpace(string(msg)), res.StatusCode)
		return
	}
	var pong nodePong
	if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&pong) != nil || !pong.Node.valid() {
		http.Error(w, "The other machine answered with an invalid identity.", 502)
		return
	}
	h.learnNodes(r.Context(), []Node{pong.Node}, pong.Node.ID)
	h.learnNodes(r.Context(), pong.Nodes, pong.Node.ID)
	h.markNode(pong.Node.ID, "", time.Now())
	writeJSON(w, 200, map[string]any{"node": pong.Node, "known": len(pong.Nodes) + 1})
}

// accept serves POST /api/v1/nodes/accept. It needs no sign-in: the one-time
// code authorizes it, and the new machine proves it holds its key.
func (h *APIHandler) accept(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code  string `json:"code"`
		Node  Node   `json:"node"`
		Proof []byte `json:"proof"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req) != nil {
		http.Error(w, "invalid join request", 400)
		return
	}
	key := hashHex(normalizeCode(req.Code))
	var inv map[string]time.Time
	if h.store.Document(r.Context(), "join-invite", key, &inv) != nil || time.Now().After(inv["expires"]) {
		http.Error(w, "That join code is not valid or has expired. Run mace join --invite again.", 403)
		return
	}
	if !req.Node.valid() || !ed25519.Verify(req.Node.PublicKey, joinProof(req.Code, req.Node.ID), req.Proof) {
		http.Error(w, "The joining machine did not prove its identity.", 403)
		return
	}
	self := h.selfNode()
	if req.Node.ID == self.ID {
		http.Error(w, "A machine cannot join itself.", 400)
		return
	}
	// The code is spent before the call back, and only one request can spend it.
	if spent, err := h.store.DeleteDocument(r.Context(), "join-invite", key); err != nil || !spent {
		http.Error(w, "That join code was already used. Run mace join --invite again.", 403)
		return
	}
	var back Node
	if err := getJSON(r.Context(), req.Node.URL+"/api/v1/node/identity", &back); err != nil || back.ID != req.Node.ID {
		http.Error(w, "This machine could not reach the new one at "+req.Node.URL+". Check that MACE and Tailscale run there, then get a new code.", 400)
		return
	}
	others := h.knownNodes(r.Context())
	h.learnNodes(r.Context(), []Node{req.Node}, self.ID)
	h.markNode(req.Node.ID, "", time.Now())
	go h.announce(req.Node, others)
	writeJSON(w, 200, nodePong{Node: self, Nodes: others})
}

// announce tells every other known node about a new one.
func (h *APIHandler) announce(n Node, to []Node) {
	for _, peer := range to {
		if peer.ID == n.ID {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := h.nodeRequest(ctx, peer, "POST", "/api/v1/node/announce", map[string][]Node{"nodes": {n}}, nil); err != nil {
			log.Printf("[MACE] Could not introduce %s to %s; it will learn on the next check: %v", n.Name, peer.Name, err)
		}
		cancel()
	}
}

// announced serves POST /api/v1/node/announce from a joined node.
func (h *APIHandler) announced(w http.ResponseWriter, r *http.Request) {
	from, body, err := h.verifyNodeRequest(r)
	if err != nil {
		http.Error(w, err.Error(), 401)
		return
	}
	var req struct {
		Nodes []Node `json:"nodes"`
	}
	if json.Unmarshal(body, &req) != nil {
		http.Error(w, "invalid announcement", 400)
		return
	}
	h.learnNodes(r.Context(), req.Nodes, from.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ping serves GET /api/v1/node/ping from a joined node, answering with this
// machine's identity and the nodes it knows.
func (h *APIHandler) ping(w http.ResponseWriter, r *http.Request) {
	from, _, err := h.verifyNodeRequest(r)
	if err != nil {
		http.Error(w, err.Error(), 401)
		return
	}
	h.markNode(from.ID, "", time.Now())
	writeJSON(w, 200, nodePong{Node: h.selfNode(), Nodes: h.knownNodes(r.Context())})
}

// removeNode serves POST /api/v1/nodes/{id}/remove. The node is remembered as
// removed so other machines' lists do not bring it back.
func (h *APIHandler) removeNode(w http.ResponseWriter, r *http.Request) {
	id := routeParam(r, "id")
	var n Node
	if h.store.Document(r.Context(), "node", id, &n) != nil {
		http.Error(w, "That machine is not joined here.", 404)
		return
	}
	h.store.PutDocument(r.Context(), "node-removed", id, map[string]time.Time{"at": time.Now().UTC()})
	h.store.DeleteDocument(r.Context(), "node", id)
	h.audit("you", "NODE_REMOVED", id, map[string]any{"name": n.Name})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// checkNodes pings every known node once.
func (h *APIHandler) checkNodes(ctx context.Context) {
	var wg sync.WaitGroup
	for _, n := range h.knownNodes(ctx) {
		wg.Add(1)
		go func(n Node) {
			defer wg.Done()
			h.checkNode(ctx, n, 0)
		}(n)
	}
	wg.Wait()
}

// checkNode pings one node, records whether it answered, and learns any
// nodes it knows that this machine does not.
func (h *APIHandler) checkNode(ctx context.Context, n Node, attempt int) {
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	asked := time.Now()
	var pong nodePong
	if err := h.nodeRequest(c, n, "GET", "/api/v1/node/ping", nil, &pong); err != nil {
		var refused *nodeRefused
		if errors.As(err, &refused) && refused.status == 401 {
			// A machine that has not yet heard of this one refuses it until
			// the introduction arrives; that is not being unreachable.
			if attempt < 4 {
				time.AfterFunc(time.Duration(attempt+1)*time.Second, func() { h.checkNode(context.Background(), n, attempt+1) })
				return
			}
			h.markNode(n.ID, "It does not recognize this machine", asked)
			return
		}
		h.markNode(n.ID, "Unreachable", asked)
		return
	}
	if pong.Node.ID != n.ID {
		h.markNode(n.ID, "A different machine answers at "+n.URL, asked)
		return
	}
	h.markNode(n.ID, "", asked)
	h.learnNodes(ctx, pong.Nodes, n.ID)
	if _, err := h.syncFrom(ctx, n); err != nil {
		log.Printf("[MACE] Sync from %s: %v", n.Name, err)
	}
}

func (h *APIHandler) watchNodes(ctx context.Context) {
	if h.records == nil {
		return
	}
	t := time.NewTicker(nodeCheckEvery)
	defer t.Stop()
	for {
		h.checkNodes(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Requests between nodes carry the sender's node ID, the time, and a
// signature over the method, path, time, and body hash.

func requestProof(method, path, at string, body []byte) []byte {
	sum := sha256.Sum256(body)
	return []byte("mace-node-request/1\n" + method + "\n" + path + "\n" + at + "\n" + hex.EncodeToString(sum[:]))
}

// nodeRequest sends a signed request to a node and decodes the answer.
func (h *APIHandler) nodeRequest(ctx context.Context, n Node, method, path string, body, out any) error {
	if h.records == nil {
		return errors.New("this machine has no node identity")
	}
	var data []byte
	if body != nil {
		var err error
		if data, err = json.Marshal(body); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, n.URL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	at := strconv.FormatInt(time.Now().UnixMilli(), 10)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mace-Node", h.records.Identity().ID)
	req.Header.Set("X-Mace-Time", at)
	req.Header.Set("X-Mace-Signature", base64.StdEncoding.EncodeToString(h.records.Identity().Sign(requestProof(method, req.URL.RequestURI(), at, data))))
	res, err := (&http.Client{Transport: peerTransport}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 1000))
		return &nodeRefused{node: n.Name, status: res.StatusCode, msg: strings.TrimSpace(string(msg))}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(res.Body, 48<<20)).Decode(out)
}

// verifyNodeRequest checks a signed request from a joined node and returns
// the node and the request body.
func (h *APIHandler) verifyNodeRequest(r *http.Request) (Node, []byte, error) {
	var n Node
	if h.store.Document(r.Context(), "node", r.Header.Get("X-Mace-Node"), &n) != nil || !n.valid() {
		return n, nil, errors.New("unknown node")
	}
	at := r.Header.Get("X-Mace-Time")
	ms, err := strconv.ParseInt(at, 10, 64)
	if err != nil || time.Since(time.UnixMilli(ms)).Abs() > signatureSkew {
		return n, nil, errors.New("the request time is missing or too far from this machine's clock")
	}
	sig, err := base64.StdEncoding.DecodeString(r.Header.Get("X-Mace-Signature"))
	if err != nil {
		return n, nil, errors.New("invalid signature")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		return n, nil, err
	}
	if !ed25519.Verify(n.PublicKey, requestProof(r.Method, r.URL.RequestURI(), at, body), sig) {
		return n, nil, errors.New("invalid signature")
	}
	return n, body, nil
}

// nodeRefused is a node's non-success answer.
type nodeRefused struct {
	node   string
	status int
	msg    string
}

func (e *nodeRefused) Error() string {
	return fmt.Sprintf("%s answered %d: %s", e.node, e.status, e.msg)
}

func getJSON(ctx context.Context, url string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	res, err := (&http.Client{Transport: peerTransport}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("answered %d", res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out)
}
