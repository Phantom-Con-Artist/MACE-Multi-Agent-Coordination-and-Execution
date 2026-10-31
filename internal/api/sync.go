package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"mace/internal/records"
	"mace/internal/storage"
)

// Sync copies records between nodes (docs/DESIGN-HUB-WORKER.md). Each node
// reports the head of every chain it holds, its own and cached ones; a node
// fetches only the part of a chain it lacks and checks every record against
// the key of the node that wrote it, whoever relayed it. Copies are cached
// read-only; each node stays the only writer of its own chain.

const chainPageSize = 500

type chainPage struct {
	Records []records.Record `json:"records"`
	More    bool             `json:"more"`
	Next    string           `json:"next,omitempty"`
}

// recordView is a record as the interface sees it.
type recordView struct {
	records.Record
	Cached bool `json:"cached,omitempty"`
}

// conversationView is a conversation merged from every node's chain, with the
// names of nodes that hold part of it and are offline now.
type conversationView struct {
	Records []recordView `json:"records"`
	Offline []string     `json:"offline"`
}

// heads serves GET /api/v1/node/heads to joined nodes.
func (h *APIHandler) heads(w http.ResponseWriter, r *http.Request) {
	if _, _, err := h.verifyNodeRequest(r); err != nil {
		http.Error(w, err.Error(), 401)
		return
	}
	list, err := h.store.AllHeads(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, list)
}

// chain serves GET /api/v1/node/chain to joined nodes: records of one node's
// chain in one conversation, walking back from head until known, returned
// oldest first, at most one page at a time.
func (h *APIHandler) chain(w http.ResponseWriter, r *http.Request) {
	if _, _, err := h.verifyNodeRequest(r); err != nil {
		http.Error(w, err.Error(), 401)
		return
	}
	q := r.URL.Query()
	node, conversation, id, known := q.Get("node"), q.Get("conversation"), q.Get("head"), q.Get("known")
	page := chainPage{Records: []records.Record{}}
	for id != "" && id != known {
		if len(page.Records) == chainPageSize {
			page.More, page.Next = true, id
			break
		}
		row, err := h.store.RecordByID(r.Context(), id)
		if err != nil || row.Node != node || row.Conversation != conversation {
			http.Error(w, "That chain is not held here.", 404)
			return
		}
		var rec records.Record
		if json.Unmarshal(row.Data, &rec) != nil {
			http.Error(w, "A stored record is unreadable.", 500)
			return
		}
		page.Records = append(page.Records, rec)
		id = row.Prev
	}
	// Walked newest first; send oldest first.
	for i, j := 0, len(page.Records)-1; i < j; i, j = i+1, j-1 {
		page.Records[i], page.Records[j] = page.Records[j], page.Records[i]
	}
	writeJSON(w, 200, page)
}

// nodeKey returns the public key of a joined node or this machine.
func (h *APIHandler) nodeKey(ctx context.Context, id string) []byte {
	if self := h.selfNode(); self.ID == id {
		return self.PublicKey
	}
	var n Node
	if h.store.Document(ctx, "node", id, &n) == nil && n.valid() {
		return n.PublicKey
	}
	return nil
}

var syncLocks sync.Map

// syncFrom fetches every chain part a node holds that this machine lacks.
func (h *APIHandler) syncFrom(ctx context.Context, peer Node) (int, error) {
	// One sync per peer at a time. A sync waits rather than skips, because a
	// running one may have read the heads before the newest records existed.
	lock, _ := syncLocks.LoadOrStore(h.selfNode().ID+"|"+peer.ID, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()
	var heads []storage.ChainHead
	if err := h.nodeRequest(ctx, peer, "GET", "/api/v1/node/heads", nil, &heads); err != nil {
		return 0, err
	}
	self := h.selfNode().ID
	added := 0
	var problems []error
	for _, head := range heads {
		if head.Node == self {
			continue // This machine is the only writer of its own chains.
		}
		key := h.nodeKey(ctx, head.Node)
		if key == nil {
			continue // Not introduced yet; the next round picks it up.
		}
		local, err := h.store.RecordHead(ctx, head.Node, head.Conversation)
		if err != nil {
			return added, err
		}
		if local == head.Head {
			continue
		}
		if _, err := h.store.RecordByID(ctx, head.Head); err == nil {
			continue // The relaying node's copy is older than this one.
		}
		n, err := h.fetchChain(ctx, peer, head, local, key)
		added += n
		if n > 0 {
			h.events.publish("records", map[string]string{"conversation": head.Conversation, "node": head.Node})
		}
		if err != nil {
			problems = append(problems, err)
		}
	}
	return added, errors.Join(problems...)
}

func (h *APIHandler) fetchChain(ctx context.Context, peer Node, head storage.ChainHead, local string, key []byte) (int, error) {
	list := []records.Record{}
	next := head.Head
	for {
		q := url.Values{"node": {head.Node}, "conversation": {head.Conversation}, "head": {next}, "known": {local}}
		var page chainPage
		if err := h.nodeRequest(ctx, peer, "GET", "/api/v1/node/chain?"+q.Encode(), nil, &page); err != nil {
			return 0, err
		}
		list = append(page.Records, list...)
		if !page.More {
			break
		}
		if len(list) > 100*chainPageSize {
			return 0, fmt.Errorf("the chain of %s in %s is too long to fetch at once", short(head.Node), head.Conversation)
		}
		next = page.Next
	}
	if len(list) == 0 {
		return 0, nil
	}
	if list[0].Prev != local {
		return 0, fmt.Errorf("%s holds a history of node %s in %s that does not continue the one here; the copy here is kept", peer.Name, short(head.Node), head.Conversation)
	}
	rows := make([]storage.RecordRow, 0, len(list))
	prev := local
	for _, r := range list {
		if r.Node != head.Node || r.Conversation != head.Conversation || r.Prev != prev {
			return 0, fmt.Errorf("%s sent a broken chain for %s", peer.Name, head.Conversation)
		}
		if err := records.Verify(r, key); err != nil {
			return 0, fmt.Errorf("%s sent a record that failed verification: %w", peer.Name, err)
		}
		data, _ := json.Marshal(r)
		rows = append(rows, storage.RecordRow{ID: r.ID, Node: r.Node, Conversation: r.Conversation, Wall: r.Clock.Wall, Count: r.Clock.Count, Prev: r.Prev, Kind: r.Kind, Cached: true, Data: data})
		h.records.Observe(r.Clock)
		prev = r.ID
	}
	if prev != head.Head {
		return 0, fmt.Errorf("%s sent a chain that does not end at its reported head", peer.Name)
	}
	if err := h.store.AppendRecords(ctx, rows, local); err != nil {
		if errors.Is(err, storage.ErrHeadMoved) {
			return 0, nil // Another sync stored it first.
		}
		return 0, err
	}
	return len(rows), nil
}

func short(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// syncAll syncs from every available node.
func (h *APIHandler) syncAll(ctx context.Context) {
	var wg sync.WaitGroup
	for _, n := range h.knownNodes(ctx) {
		if !h.nodeAvailable(n.ID) {
			continue
		}
		wg.Add(1)
		go func(n Node) {
			defer wg.Done()
			if added, err := h.syncFrom(ctx, n); err != nil {
				log.Printf("[MACE] Sync from %s: %v", n.Name, err)
			} else if added > 0 {
				log.Printf("[MACE] Synced %d records from %s.", added, n.Name)
			}
		}(n)
	}
	wg.Wait()
}

func (h *APIHandler) nodeAvailable(id string) bool {
	h.nodeHealth.mu.Lock()
	defer h.nodeHealth.mu.Unlock()
	seen, ok := h.nodeHealth.seen[id]
	_, failing := h.nodeHealth.fail[id]
	return ok && !failing && time.Since(seen) < 3*nodeCheckEvery
}

// syncNow serves POST /api/v1/records/sync.
func (h *APIHandler) syncNow(w http.ResponseWriter, r *http.Request) {
	h.syncAll(r.Context())
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// conversationRecords serves GET /api/v1/records/{conversation}: every record
// held here for the conversation, in causal order.
func (h *APIHandler) conversationRecords(w http.ResponseWriter, r *http.Request) {
	if h.records == nil {
		http.Error(w, "Records are off on this machine; see the log.", 503)
		return
	}
	conversation := routeParam(r, "conversation")
	rows, err := h.store.ConversationRecords(r.Context(), conversation)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	cached := map[string]bool{}
	nodes := map[string]bool{}
	for _, row := range rows {
		cached[row.ID] = row.Cached
		nodes[row.Node] = true
	}
	list, err := h.records.Conversation(r.Context(), conversation)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	view := conversationView{Records: make([]recordView, 0, len(list)), Offline: []string{}}
	for _, rec := range list {
		view.Records = append(view.Records, recordView{Record: rec, Cached: cached[rec.ID]})
	}
	self := h.selfNode().ID
	for _, n := range h.knownNodes(r.Context()) {
		if nodes[n.ID] && n.ID != self && !h.nodeAvailable(n.ID) {
			view.Offline = append(view.Offline, n.Name)
		}
	}
	writeJSON(w, 200, view)
}
