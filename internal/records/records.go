// Package records is MACE's storage unit across nodes. Every message, setting,
// receipt, or file manifest a node writes is a record: named by the SHA-256 of
// its canonical envelope, signed by the node, and chained to the node's
// previous record in the same conversation. Each node is the only writer of
// its own records, so histories from many nodes merge without conflicts.
// See docs/DESIGN-HUB-WORKER.md.
package records

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"time"
)

// Version is the canonical encoding version. Changing the envelope or its
// encoding needs a new version so existing hashes stay valid.
const Version = 1

// Record is one immutable entry. ID and Sig are derived from the other fields.
type Record struct {
	V            int             `json:"v"`
	Node         string          `json:"node"`
	Author       string          `json:"author"`
	Conversation string          `json:"conversation"`
	Clock        Clock           `json:"clock"`
	Prev         string          `json:"prev,omitempty"`
	Seen         []string        `json:"seen,omitempty"`
	Kind         string          `json:"kind"`
	Body         json.RawMessage `json:"body"`
	ID           string          `json:"id,omitempty"`
	Sig          []byte          `json:"sig,omitempty"`
}

// envelope is the hashed part of a record, in a fixed field order.
type envelope struct {
	V            int             `json:"v"`
	Node         string          `json:"node"`
	Author       string          `json:"author"`
	Conversation string          `json:"conversation"`
	Clock        Clock           `json:"clock"`
	Prev         string          `json:"prev"`
	Seen         []string        `json:"seen"`
	Kind         string          `json:"kind"`
	Body         json.RawMessage `json:"body"`
}

// Hash returns the record's ID: SHA-256 over a version prefix and the
// canonical envelope, in lowercase hex.
func Hash(r Record) (string, error) {
	b, err := canonical(r)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func canonical(r Record) ([]byte, error) {
	if r.V != Version {
		return nil, fmt.Errorf("unsupported record version %d", r.V)
	}
	body, err := canonicalBody(r.Body)
	if err != nil {
		return nil, err
	}
	seen := slices.Clone(r.Seen)
	sort.Strings(seen)
	seen = slices.Compact(seen)
	if seen == nil {
		seen = []string{}
	}
	var buf bytes.Buffer
	buf.WriteString("mace-record/1\n")
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	err = enc.Encode(envelope{V: r.V, Node: r.Node, Author: r.Author, Conversation: r.Conversation, Clock: r.Clock, Prev: r.Prev, Seen: seen, Kind: r.Kind, Body: body})
	return buf.Bytes(), err
}

// canonicalBody rewrites JSON with sorted keys and no spacing. Numbers keep
// their exact text, so large integers and decimals hash the same everywhere.
func canonicalBody(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage("null"), nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("record body is not JSON: %w", err)
	}
	if dec.More() {
		return nil, errors.New("record body has trailing data")
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// Identity is a node's signing key. The node ID is derived from the public key.
type Identity struct {
	ID      string
	Public  ed25519.PublicKey
	private ed25519.PrivateKey
}

// NodeID names a node by the first 16 bytes of its public key's SHA-256.
func NodeID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:16])
}

// NewIdentity creates a key that lives only in memory, for tests and
// in-memory stores.
func NewIdentity() *Identity {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	return &Identity{ID: NodeID(pub), Public: pub, private: priv}
}

// LoadIdentity reads the node key at path, creating it on first use. The file
// holds the 32-byte seed and is readable only by its owner.
func LoadIdentity(path string) (*Identity, error) {
	seed, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		seed = make([]byte, ed25519.SeedSize)
		if _, err := rand.Read(seed); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return nil, err
		}
		_, werr := f.Write(seed)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return nil, werr
		}
	} else if err != nil {
		return nil, err
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("node key %s is damaged; move it aside to create a new node identity", path)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)
	return &Identity{ID: NodeID(pub), Public: pub, private: priv}, nil
}

// Seal fills in the version, ID, and signature. The record's Node must be this
// identity's ID.
func (id *Identity) Seal(r Record) (Record, error) {
	r.V = Version
	if r.Node != id.ID {
		return r, fmt.Errorf("node %s cannot seal a record for node %s", id.ID, r.Node)
	}
	body, err := canonicalBody(r.Body)
	if err != nil {
		return r, err
	}
	r.Body = body
	r.Seen = slices.Compact(slices.Sorted(slices.Values(r.Seen)))
	if r.ID, err = Hash(r); err != nil {
		return r, err
	}
	sum, _ := hex.DecodeString(r.ID)
	r.Sig = ed25519.Sign(id.private, sum)
	return r, nil
}

// Verify checks that a record's ID matches its content, that it names the
// node owning pub, and that the node signed it.
func Verify(r Record, pub ed25519.PublicKey) error {
	if r.Node != NodeID(pub) {
		return fmt.Errorf("record %s names node %s, not the given key", short(r.ID), r.Node)
	}
	want, err := Hash(r)
	if err != nil {
		return err
	}
	if want != r.ID {
		return fmt.Errorf("record %s does not match its content", short(r.ID))
	}
	sum, _ := hex.DecodeString(r.ID)
	if !ed25519.Verify(pub, sum, r.Sig) {
		return fmt.Errorf("record %s has an invalid signature", short(r.ID))
	}
	return nil
}

// VerifyChain checks one node's records in one conversation, oldest first:
// each is valid and links to the one before it.
func VerifyChain(chain []Record, pub ed25519.PublicKey) error {
	prev := ""
	for i, r := range chain {
		if err := Verify(r, pub); err != nil {
			return err
		}
		if i > 0 && r.Prev != prev {
			return fmt.Errorf("record %s does not follow %s; a record is missing", short(r.ID), short(prev))
		}
		prev = r.ID
	}
	return nil
}

func short(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// Clock is a hybrid logical clock reading: wall time in milliseconds plus a
// counter for events within the same millisecond or behind a faster clock.
type Clock struct {
	Wall  int64  `json:"wall"`
	Count uint32 `json:"count"`
}

// Less orders clock readings.
func (c Clock) Less(o Clock) bool {
	return c.Wall < o.Wall || (c.Wall == o.Wall && c.Count < o.Count)
}

// HLC issues clock readings that never move backwards, even when the wall
// clock does, and that move past any clock observed from another node.
type HLC struct {
	mu   sync.Mutex
	last Clock
	now  func() time.Time
}

// Now returns the next reading.
func (h *HLC) Now() Clock { return h.Stamp(h.wall()) }

// Stamp returns the next reading at or after the given wall time, used to
// keep original times when older data is turned into records.
func (h *HLC) Stamp(wall int64) Clock {
	h.mu.Lock()
	defer h.mu.Unlock()
	if wall > h.last.Wall {
		h.last = Clock{Wall: wall}
	} else {
		h.last.Count++
	}
	return h.last
}

// Observe moves the clock past a reading from another record.
func (h *HLC) Observe(c Clock) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.last.Less(c) {
		h.last = c
	}
}

func (h *HLC) wall() int64 {
	if h.now == nil {
		return time.Now().UnixMilli()
	}
	return h.now().UnixMilli()
}

// Order sorts records so each follows everything it links to through Prev and
// Seen, breaking ties by clock, then node, then ID. Links to records outside
// the set are ignored, so a partial history still orders.
func Order(list []Record) []Record {
	byID := make(map[string]Record, len(list))
	for _, r := range list {
		byID[r.ID] = r
	}
	waiting := map[string]int{}
	next := map[string][]string{}
	for id, r := range byID {
		for _, dep := range append([]string{r.Prev}, r.Seen...) {
			if _, ok := byID[dep]; ok && dep != id {
				waiting[id]++
				next[dep] = append(next[dep], id)
			}
		}
	}
	before := func(a, b Record) bool {
		if a.Clock != b.Clock {
			return a.Clock.Less(b.Clock)
		}
		if a.Node != b.Node {
			return a.Node < b.Node
		}
		return a.ID < b.ID
	}
	ready := []Record{}
	for id, r := range byID {
		if waiting[id] == 0 {
			ready = append(ready, r)
		}
	}
	out := make([]Record, 0, len(byID))
	for len(ready) > 0 {
		sort.Slice(ready, func(i, j int) bool { return before(ready[i], ready[j]) })
		r := ready[0]
		ready = ready[1:]
		out = append(out, r)
		for _, id := range next[r.ID] {
			if waiting[id]--; waiting[id] == 0 {
				ready = append(ready, byID[id])
			}
		}
	}
	// A cycle cannot be built from hashes, but a forged set could claim one;
	// anything left is appended by clock rather than dropped.
	if len(out) < len(byID) {
		rest := []Record{}
		for id, r := range byID {
			if waiting[id] > 0 {
				rest = append(rest, r)
			}
		}
		sort.Slice(rest, func(i, j int) bool { return before(rest[i], rest[j]) })
		out = append(out, rest...)
	}
	return out
}
