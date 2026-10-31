package api

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Tailscale already proves which account every device on the tailnet belongs
// to, so the owner's machines trust each other without codes. Every MACE
// keeps looking for MACE on the owner's other devices and joins the ones it
// finds, so whichever starts first, a machine that comes online later joins
// by itself. Phones opening the hub's tailnet HTTPS address are signed in by
// the identity tailscale serve attaches to the request. Only devices logged
// in to the owner's own Tailscale account count: other users on a shared
// tailnet, devices shared in from elsewhere, and tagged devices do not.
// MACE_TAILSCALE_TRUST=off turns all of this off; join codes and QR sign-in
// still work.

type tailnetPeer struct {
	Host   string
	DNS    string
	IPs    []string
	User   string
	Online bool
	Tagged bool
	// URLs, when set, are where MACE may answer on this device (tests).
	URLs []string
}

type tailnetView struct {
	User  string
	Login string
	Peers []tailnetPeer
}

type tailnetState struct {
	mu     sync.Mutex
	at     time.Time
	view   tailnetView
	err    error
	probed map[string]time.Time
	// discovering keeps one search for machines running at a time.
	discovering atomic.Bool
	// source replaces the tailscale command, for tests.
	source func() (tailnetView, error)
}

func tailscaleTrust() bool { return os.Getenv("MACE_TAILSCALE_TRUST") != "off" }

// tailnet is this device's view of the owner's tailnet, refreshed at most
// every ten seconds.
func (h *APIHandler) tailnet() (tailnetView, error) {
	h.tail.mu.Lock()
	defer h.tail.mu.Unlock()
	if time.Since(h.tail.at) < 10*time.Second {
		return h.tail.view, h.tail.err
	}
	source := h.tail.source
	if source == nil {
		source = readTailscaleStatus
	}
	h.tail.view, h.tail.err = source()
	h.tail.at = time.Now()
	return h.tail.view, h.tail.err
}

func readTailscaleStatus() (tailnetView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tailscale", "status", "--json").Output()
	if err != nil {
		return tailnetView{}, errors.New("Tailscale is not running here")
	}
	return parseTailscaleStatus(out)
}

func parseTailscaleStatus(out []byte) (tailnetView, error) {
	type node struct {
		HostName     string
		DNSName      string
		TailscaleIPs []string
		UserID       int64
		Online       bool
		Tags         []string
	}
	var st struct {
		BackendState string
		Self         node
		Peer         map[string]node
		User         map[string]struct{ LoginName string }
	}
	if err := json.Unmarshal(out, &st); err != nil {
		return tailnetView{}, err
	}
	if st.BackendState != "Running" {
		return tailnetView{}, errors.New("Tailscale is " + strings.ToLower(st.BackendState))
	}
	if len(st.Self.Tags) > 0 {
		return tailnetView{}, errors.New("this device is tagged, so it belongs to no one's account")
	}
	self := strconv.FormatInt(st.Self.UserID, 10)
	v := tailnetView{User: self, Login: st.User[self].LoginName}
	for _, p := range st.Peer {
		v.Peers = append(v.Peers, tailnetPeer{Host: p.HostName, DNS: strings.TrimSuffix(p.DNSName, "."), IPs: p.TailscaleIPs, User: strconv.FormatInt(p.UserID, 10), Online: p.Online, Tagged: len(p.Tags) > 0})
	}
	return v, nil
}

// owned reports whether a peer belongs to the owner's own account.
func (v tailnetView) owned(p tailnetPeer) bool {
	return v.User != "" && p.User == v.User && !p.Tagged
}

// ownerRequest reports whether a request comes from a device on the owner's
// own Tailscale account: through tailscale serve on this machine, which names
// the user, or directly from one of the owner's devices.
func (h *APIHandler) ownerRequest(r *http.Request) bool {
	if !tailscaleTrust() {
		return false
	}
	v, err := h.tailnet()
	if err != nil {
		return false
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if parsed := net.ParseIP(ip); parsed != nil && parsed.IsLoopback() {
		if login := r.Header.Get("Tailscale-User-Login"); login != "" && v.Login != "" {
			return strings.EqualFold(login, v.Login)
		}
	}
	for _, p := range v.Peers {
		if v.owned(p) && slices.Contains(p.IPs, ip) {
			return true
		}
	}
	return false
}

// postJSON sends a JSON body and decodes the answer, reporting refusals
// with the other machine's message.
func postJSON(ctx context.Context, url string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := (&http.Client{Transport: peerTransport}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 1000))
		return errors.New(strings.TrimSpace(string(msg)))
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out)
}

func autojoinProof(to, from string) []byte {
	return []byte("mace-autojoin/1\n" + to + "\n" + from)
}

// autojoin serves POST /api/v1/nodes/autojoin: a MACE on another of the
// owner's devices asks to join. No code is needed; Tailscale vouches for the
// device's owner, and the new machine proves it holds its key.
func (h *APIHandler) autojoin(w http.ResponseWriter, r *http.Request) {
	if !h.ownerRequest(r) {
		http.Error(w, "That device is not on your Tailscale account. Join it with a code: mace join --invite.", 403)
		return
	}
	var req struct {
		Node  Node   `json:"node"`
		Proof []byte `json:"proof"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req) != nil {
		http.Error(w, "invalid join request", 400)
		return
	}
	self := h.selfNode()
	if !req.Node.valid() || req.Node.ID == self.ID || !ed25519.Verify(req.Node.PublicKey, autojoinProof(self.ID, req.Node.ID), req.Proof) {
		http.Error(w, "The joining machine did not prove its identity.", 403)
		return
	}
	var gone struct{}
	if h.store.Document(r.Context(), "node-removed", req.Node.ID, &gone) == nil {
		http.Error(w, "You removed this machine. Join it again with a code: mace join --invite.", 403)
		return
	}
	var back Node
	if err := getJSON(r.Context(), req.Node.URL+"/api/v1/node/identity", &back); err != nil || back.ID != req.Node.ID {
		http.Error(w, "This machine could not reach the new one at "+req.Node.URL+".", 400)
		return
	}
	others := h.knownNodes(r.Context())
	if added := h.learnNodes(r.Context(), []Node{req.Node}, self.ID); len(added) > 0 {
		log.Printf("[MACE] %s joined through your Tailscale account.", req.Node.Name)
		go h.announce(req.Node, others)
	}
	h.markNode(req.Node.ID, "", time.Now())
	writeJSON(w, 200, nodePong{Node: self, Nodes: others})
}

// discoverMachines looks for MACE on the owner's other online devices and
// joins each one it finds that it does not know yet.
func (h *APIHandler) discoverMachines(ctx context.Context) {
	if !tailscaleTrust() || h.records == nil || !h.tail.discovering.CompareAndSwap(false, true) {
		return
	}
	defer h.tail.discovering.Store(false)
	v, err := h.tailnet()
	if err != nil {
		return
	}
	known := h.knownNodes(ctx)
	self := h.selfNode()
	for _, p := range v.Peers {
		if !v.owned(p) || !p.Online || knownDevice(known, p) {
			continue
		}
		key := p.Host + strings.Join(p.IPs, ",")
		h.tail.mu.Lock()
		if h.tail.probed == nil {
			h.tail.probed = map[string]time.Time{}
		}
		recent := time.Since(h.tail.probed[key]) < time.Minute
		if !recent {
			h.tail.probed[key] = time.Now()
		}
		h.tail.mu.Unlock()
		if recent {
			continue
		}
		for _, base := range h.candidateURLs(p) {
			if h.joinDevice(ctx, self, base) {
				break
			}
		}
	}
}

// knownDevice reports whether a device already runs a joined MACE.
func knownDevice(known []Node, p tailnetPeer) bool {
	for _, n := range known {
		u, err := url.Parse(n.URL)
		if err != nil {
			continue
		}
		for _, addr := range append([]string{p.DNS}, p.IPs...) {
			if addr != "" && u.Hostname() == addr {
				return true
			}
		}
		if slices.Contains(p.URLs, n.URL) {
			return true
		}
	}
	return false
}

// candidateURLs are where MACE may answer on a device: its tailnet HTTPS
// name (mace-server), then the default port and this machine's port.
func (h *APIHandler) candidateURLs(p tailnetPeer) []string {
	if len(p.URLs) > 0 {
		return p.URLs
	}
	out := []string{}
	if p.DNS != "" {
		out = append(out, "https://"+p.DNS)
	}
	ports := []string{"8080"}
	if h.port != 0 && h.port != 8080 {
		ports = append(ports, strconv.Itoa(h.port))
	}
	for _, ip := range p.IPs {
		if strings.Contains(ip, ":") {
			continue // IPv4 is enough to reach a device
		}
		for _, port := range ports {
			out = append(out, "http://"+net.JoinHostPort(ip, port))
		}
	}
	return out
}

// joinDevice asks the MACE at base to join, and reports whether it did.
func (h *APIHandler) joinDevice(ctx context.Context, self Node, base string) bool {
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	var them Node
	err := getJSON(c, base+"/api/v1/node/identity", &them)
	cancel()
	if err != nil || !them.valid() || them.ID == self.ID {
		return false
	}
	var gone struct{}
	if h.store.Document(ctx, "node-removed", them.ID, &gone) == nil {
		return true // the owner removed it; do not bring it back
	}
	body := map[string]any{"node": self, "proof": h.records.Identity().Sign(autojoinProof(them.ID, self.ID))}
	c, cancel = context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var pong nodePong
	if err := postJSON(c, base+"/api/v1/nodes/autojoin", body, &pong); err != nil {
		log.Printf("[MACE] Found MACE at %s but could not join it: %v", base, err)
		return true
	}
	if pong.Node.ID != them.ID {
		return true
	}
	// The address that answered is the one known to work from here.
	pong.Node.URL = base
	h.learnNodes(ctx, []Node{pong.Node}, them.ID)
	h.markNode(them.ID, "", time.Now())
	h.learnNodes(ctx, pong.Nodes, them.ID)
	log.Printf("[MACE] Joined %s through your Tailscale account.", pong.Node.Name)
	return true
}
