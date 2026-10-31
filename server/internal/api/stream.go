package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// hub fans out workspace events to connected browsers over server-sent events.
// A ring of recent events lets a reconnecting client resume with Last-Event-ID.
// Event IDs carry the hub's epoch, so IDs from before a restart cause a reset
// instead of silently skipping or repeating events.
type hub struct {
	mu    sync.Mutex
	epoch string
	seq   uint64
	ring  []hubEvent
	subs  map[chan hubEvent]struct{}
}

type hubEvent struct {
	Seq  uint64
	Type string
	Data []byte
}

const hubRing = 2000

func newHub() *hub {
	return &hub{epoch: strconv.FormatInt(time.Now().UnixNano(), 36), subs: map[chan hubEvent]struct{}{}}
}

func (b *hub) publish(typ string, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	ev := hubEvent{Seq: b.seq, Type: typ, Data: raw}
	b.ring = append(b.ring, ev)
	if len(b.ring) > hubRing {
		b.ring = b.ring[len(b.ring)-hubRing:]
	}
	for ch := range b.subs {
		select {
		case ch <- ev:
		default:
			// A client too slow to keep up is disconnected and resumes from its
			// last event ID, which is cheaper than buffering without bound.
			delete(b.subs, ch)
			close(ch)
		}
	}
}

// subscribe returns events after lastID and a live channel. ok is false when
// lastID cannot be resumed and the client must reload its state.
func (b *hub) subscribe(lastID string) (backlog []hubEvent, ok bool, ch chan hubEvent, cancel func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	ok = true
	if lastID != "" {
		epoch, seqText, found := strings.Cut(lastID, "-")
		after, err := strconv.ParseUint(seqText, 10, 64)
		switch {
		case !found || err != nil || epoch != b.epoch || after > b.seq:
			ok = false
		case after < b.seq:
			if len(b.ring) == 0 || b.ring[0].Seq > after+1 {
				ok = false
			} else {
				for _, ev := range b.ring {
					if ev.Seq > after {
						backlog = append(backlog, ev)
					}
				}
			}
		}
	}
	ch = make(chan hubEvent, 512)
	b.subs[ch] = struct{}{}
	cancel = func() {
		b.mu.Lock()
		if _, live := b.subs[ch]; live {
			delete(b.subs, ch)
			close(ch)
		}
		b.mu.Unlock()
	}
	return backlog, ok, ch, cancel
}

func (b *hub) id(seq uint64) string { return b.epoch + "-" + strconv.FormatUint(seq, 10) }

// stream serves GET /api/v1/stream.
func (h *APIHandler) stream(w http.ResponseWriter, r *http.Request) {
	flusher, canFlush := w.(http.Flusher)
	if !canFlush {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	lastID := r.Header.Get("Last-Event-ID")
	if lastID == "" {
		lastID = r.URL.Query().Get("last")
	}
	backlog, ok, ch, cancel := h.events.subscribe(lastID)
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	write := func(ev hubEvent) error {
		_, err := fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", h.events.id(ev.Seq), ev.Type, ev.Data)
		return err
	}
	fmt.Fprintf(w, "retry: 2000\n\n")
	if !ok || lastID == "" {
		h.events.mu.Lock()
		current := hubEvent{Seq: h.events.seq, Type: "reset", Data: []byte(`{}`)}
		h.events.mu.Unlock()
		write(current)
	}
	for _, ev := range backlog {
		if write(ev) != nil {
			return
		}
	}
	flusher.Flush()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, open := <-ch:
			if !open {
				return
			}
			if write(ev) != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
