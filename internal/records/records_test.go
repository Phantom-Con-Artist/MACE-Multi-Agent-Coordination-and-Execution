package records

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fixedClock(ms ...int64) func() time.Time {
	i := 0
	return func() time.Time {
		t := time.UnixMilli(ms[min(i, len(ms)-1)])
		i++
		return t
	}
}

func TestHashIsStableAndCoversEveryField(t *testing.T) {
	id := NewIdentity()
	r := Record{Node: id.ID, Author: "you", Conversation: "c1", Clock: Clock{Wall: 1000, Count: 2}, Kind: "message", Body: json.RawMessage(`{"text":"ok","b":1}`)}
	a, err := id.Seal(r)
	if err != nil {
		t.Fatal(err)
	}
	// Key order and spacing in the body do not change the hash.
	r.Body = json.RawMessage(`{ "b": 1, "text": "ok" }`)
	b, _ := id.Seal(r)
	if a.ID != b.ID {
		t.Fatalf("canonical bodies should hash the same: %s %s", a.ID, b.ID)
	}
	// Identical text in a different envelope is a different record.
	r.Clock.Count = 3
	c, _ := id.Seal(r)
	if c.ID == a.ID {
		t.Fatal("a different clock must change the ID")
	}
	if err := Verify(a, id.Public); err != nil {
		t.Fatalf("a sealed record should verify: %v", err)
	}
	tampered := a
	tampered.Body = json.RawMessage(`{"b":1,"text":"no"}`)
	if Verify(tampered, id.Public) == nil {
		t.Fatal("changed content must fail verification")
	}
	other := NewIdentity()
	if Verify(a, other.Public) == nil {
		t.Fatal("another node's key must fail verification")
	}
}

func TestBodyNumbersKeepTheirExactText(t *testing.T) {
	got, err := canonicalBody(json.RawMessage(`{"n":12345678901234567890,"f":1.50}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"f":1.50,"n":12345678901234567890}` {
		t.Fatalf("canonical body %s", got)
	}
}

func TestClockNeverGoesBackwards(t *testing.T) {
	h := &HLC{now: fixedClock(5000, 4000, 4000, 6000)}
	a, b, c, d := h.Now(), h.Now(), h.Now(), h.Now()
	if !a.Less(b) || !b.Less(c) || !c.Less(d) {
		t.Fatalf("clock went backwards: %v %v %v %v", a, b, c, d)
	}
	// A remote clock ahead of ours moves ours forward.
	h.Observe(Clock{Wall: 9000, Count: 4})
	if e := h.Now(); !(Clock{Wall: 9000, Count: 4}).Less(e) {
		t.Fatalf("observed clock not respected: %v", e)
	}
}

func TestOrderFollowsCausesDespiteClockSkew(t *testing.T) {
	laptop, pc := NewIdentity(), NewIdentity()
	q, _ := laptop.Seal(Record{Node: laptop.ID, Author: "you", Conversation: "g", Clock: Clock{Wall: 2000}, Kind: "message", Body: json.RawMessage(`{"text":"question"}`)})
	// The PC's clock runs behind, but its reply has seen the question.
	a, _ := pc.Seal(Record{Node: pc.ID, Author: "agy", Conversation: "g", Clock: Clock{Wall: 1000}, Seen: []string{q.ID}, Kind: "message", Body: json.RawMessage(`{"text":"answer"}`)})
	// An unrelated earlier note sorts by clock.
	n, _ := laptop.Seal(Record{Node: laptop.ID, Author: "mace", Conversation: "g", Clock: Clock{Wall: 500}, Kind: "note", Body: json.RawMessage(`{"text":"note"}`)})
	got := Order([]Record{a, q, n})
	if got[0].ID != n.ID || got[1].ID != q.ID || got[2].ID != a.ID {
		t.Fatalf("order: %s %s %s", got[0].Kind, got[1].Body, got[2].Body)
	}
}

func TestChainDetectsGapsAndForks(t *testing.T) {
	id := NewIdentity()
	var chain []Record
	prev := ""
	for i := range 3 {
		r, _ := id.Seal(Record{Node: id.ID, Author: "you", Conversation: "c", Clock: Clock{Wall: int64(i + 1)}, Prev: prev, Kind: "message", Body: json.RawMessage(`{}`)})
		chain = append(chain, r)
		prev = r.ID
	}
	if err := VerifyChain(chain, id.Public); err != nil {
		t.Fatalf("a whole chain should verify: %v", err)
	}
	if VerifyChain([]Record{chain[0], chain[2]}, id.Public) == nil {
		t.Fatal("a missing link must be reported")
	}
}

func TestIdentityPersistsPrivately(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node.key")
	a, err := LoadIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := LoadIdentity(path)
	if err != nil || a.ID != b.ID {
		t.Fatalf("identity should survive a restart: %v %s %s", err, a.ID, b.ID)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("the key file must be private, got %v", info.Mode().Perm())
	}
}
