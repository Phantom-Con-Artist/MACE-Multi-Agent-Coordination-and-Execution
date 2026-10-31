package api

import (
	"context"
	"encoding/json"
	"testing"

	"mace/internal/records"
)

func joinAll(t *testing.T, hub *machine, others ...*machine) {
	t.Helper()
	for _, m := range others {
		if c := m.owner(t, "POST", "/api/v1/nodes/join", map[string]string{"url": hub.ts.URL, "code": hub.invite(t)}, nil); c != 200 {
			t.Fatalf("join: %d", c)
		}
	}
}

func (m *machine) write(t *testing.T, conversation, text string) records.Record {
	t.Helper()
	r, err := m.s.handler.records.Append(context.Background(), records.Entry{Conversation: conversation, Author: "you", Kind: "message", Body: map[string]string{"text": text}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (m *machine) view(t *testing.T, conversation string) conversationView {
	t.Helper()
	var v conversationView
	if c := m.owner(t, "GET", "/api/v1/records/"+conversation, nil, &v); c != 200 {
		t.Fatalf("view: %d", c)
	}
	return v
}

func (m *machine) syncNow(t *testing.T) {
	t.Helper()
	if c := m.owner(t, "POST", "/api/v1/records/sync", map[string]any{}, nil); c != 200 {
		t.Fatalf("sync: %d", c)
	}
}

func texts(v conversationView) []string {
	out := []string{}
	for _, r := range v.Records {
		var b struct {
			Text string `json:"text"`
		}
		json.Unmarshal(r.Body, &b)
		out = append(out, b.Text)
	}
	return out
}

func TestHubMergesEveryNodesChain(t *testing.T) {
	laptop, pc := newMachine(t, "laptop"), newMachine(t, "pc")
	joinAll(t, laptop, pc)
	laptop.write(t, "g", "question from the laptop")
	pc.write(t, "g", "answer from the pc")
	pc.write(t, "g", "more from the pc")

	laptop.syncNow(t)
	v := laptop.view(t, "g")
	if len(v.Records) != 3 || len(v.Offline) != 0 {
		t.Fatalf("the laptop should hold both chains: %v offline=%v", texts(v), v.Offline)
	}
	// Only what is new is fetched the second time.
	pc.write(t, "g", "latest")
	laptop.syncNow(t)
	if got := texts(laptop.view(t, "g")); len(got) != 4 || got[3] != "latest" {
		t.Fatalf("an incremental sync should add only the new record: %v", got)
	}
}

func TestCachedCopiesTravelThroughOtherNodes(t *testing.T) {
	laptop, pc, desk := newMachine(t, "laptop"), newMachine(t, "pc"), newMachine(t, "desk")
	joinAll(t, laptop, pc, desk)
	pc.write(t, "g", "written on the pc")
	laptop.syncNow(t)

	// The pc goes away; the desk still reads its record from the laptop's
	// cache, checked against the pc's own key.
	pc.ts.Close()
	desk.s.handler.checkNodes(context.Background())
	desk.syncNow(t)
	v := desk.view(t, "g")
	if got := texts(v); len(got) != 1 || got[0] != "written on the pc" {
		t.Fatalf("the desk should read the pc's record through the laptop: %v", got)
	}
	if len(v.Offline) != 1 || v.Offline[0] != "pc" {
		t.Fatalf("the pc should be reported offline: %v", v.Offline)
	}
	if !v.Records[0].Cached {
		t.Fatal("a record from another node is a cached copy")
	}
}

func TestSyncRejectsAlteredRecords(t *testing.T) {
	laptop, pc := newMachine(t, "laptop"), newMachine(t, "pc")
	joinAll(t, laptop, pc)
	r := pc.write(t, "g", "honest")
	// Corrupt the pc's stored copy, as a damaged or malicious node might.
	r.Body = json.RawMessage(`{"text":"forged"}`)
	data, _ := json.Marshal(r)
	pc.s.handler.store.DB().Exec(`UPDATE records SET data=? WHERE id=?`, string(data), r.ID)

	laptop.syncNow(t)
	if got := texts(laptop.view(t, "g")); len(got) != 0 {
		t.Fatalf("an altered record must not be accepted: %v", got)
	}
}
