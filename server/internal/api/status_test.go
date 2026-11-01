package api

import (
	"context"
	"strings"
	"testing"
)

func openStatuses(t *testing.T, s *Server) []Status {
	t.Helper()
	var list []Status
	if c := call(t, s, "GET", "/api/v1/statuses", nil, &list); c != 200 {
		t.Fatalf("statuses: %d", c)
	}
	return list
}

func TestStatusOfferAndPick(t *testing.T) {
	tm := newTeam(t, "ari", "bo", "cy")
	room := tm.group(t, "ari", "bo", "cy")
	tm.set("ari", "reply.txt", "[status: a machine with CUDA 12] my laptop has no GPU")
	tm.set("bo", "reply.txt", "[PASS]")
	tm.set("cy", "reply.txt", "[PASS]")
	tm.say(t, room.ID, "@agy what do you need for the training test?")
	settled(t, tm.s, room.ID)

	list := openStatuses(t, tm.s)
	if len(list) != 1 || list[0].Text != "a machine with CUDA 12" || list[0].ForName != tm.agents["ari"].Name {
		t.Fatalf("the status should be on the board: %+v", list)
	}
	id := list[0].ID
	var flaired bool
	for _, m := range tm.history(t, room.ID) {
		flaired = flaired || m.Flair == "status" && strings.Contains(m.Text, "no GPU")
	}
	if !flaired {
		t.Fatal("the message that posted the status should carry the status flair")
	}

	// bo reads the board on its next turn and offers; the offer goes to a side chat.
	tm.set("bo", "reply.txt", "[offer "+id+"] RTX 4070 with CUDA 12.4 here")
	tm.set("ari", "reply.txt", "[status pick @agy-2] great, sending the script")
	tm.say(t, room.ID, "@agy-2 anything to add?")
	settled(t, tm.s, room.ID)
	if p := tm.promptsOf(t, "bo"); !strings.Contains(p, "STATUS BOARD") || !strings.Contains(p, id) {
		t.Fatalf("other agents should see the status on the board:\n%s", p)
	}
	for _, m := range tm.history(t, room.ID) {
		if strings.Contains(m.Text, "RTX 4070") {
			t.Fatal("an offer belongs in its side chat, not the group")
		}
	}
	var rooms []Room
	call(t, tm.s, "GET", "/api/v1/rooms", nil, &rooms)
	var side string
	for _, r := range rooms {
		if r.Parent == room.ID {
			side = r.ID
		}
	}
	if side == "" {
		t.Fatal("the offer should open a side chat with the agent who asked")
	}
	settled(t, tm.s, side)
	if p := tm.promptsOf(t, "ari"); !strings.Contains(p, "YOUR STATUS "+id) || !strings.Contains(p, "@agy-2") {
		t.Fatalf("the requester should see the offers on its status:\n%s", p)
	}
	list = openStatuses(t, tm.s)
	if len(list) != 1 || !list[0].Closed || list[0].Picked != tm.s.handler.qualify(tm.agents["bo"].ID) {
		t.Fatalf("the pick should close the status with bo picked: %+v", list)
	}
	if p := tm.promptsOf(t, "cy"); strings.Count(p, "STATUS BOARD") > 1 {
		t.Fatal("a closed status should leave the board")
	}
}

func TestOnlyTheLeadPostsStatusesInATeam(t *testing.T) {
	tm := newTeam(t, "ari", "bo")
	room := tm.group(t, "ari", "bo")
	if c := call(t, tm.s, "POST", "/api/v1/chats/"+room.ID+"/policy", Policy{Mode: "team", Lead: tm.agents["ari"].ID}, nil); c != 200 {
		t.Fatalf("policy: %d", c)
	}
	tm.set("ari", "reply.txt", "[PASS]")
	tm.set("bo", "reply.txt", "[status: a GPU] need one")
	tm.say(t, room.ID, "@agy-2 go")
	settled(t, tm.s, room.ID)
	if list := openStatuses(t, tm.s); len(list) != 0 {
		t.Fatalf("a team member's status should wait for the lead: %+v", list)
	}
	tm.set("ari", "reply.txt", "[status: a machine with a GPU for @agy-2] posting for bo")
	tm.say(t, room.ID, "@agy please post it")
	settled(t, tm.s, room.ID)
	list := openStatuses(t, tm.s)
	if len(list) != 1 || list[0].For != tm.s.handler.qualify(tm.agents["bo"].ID) || list[0].Poster != tm.s.handler.qualify(tm.agents["ari"].ID) {
		t.Fatalf("the lead's status should be for bo: %+v", list)
	}
}

func TestStatusesReachLinkedMachinesAndOffersComeHome(t *testing.T) {
	a, b := newMachine(t, "laptop"), newMachine(t, "pc")
	joinAll(t, a, b)
	ctx := context.Background()
	ha, hb := a.s.handler, b.s.handler
	group := Room{ID: "room_gpu", Name: "Training", Members: []string{"session_ari"}}
	ha.store.PutDocument(ctx, "room", group.ID, group)
	if err := ha.statusPost(ctx, &group, "session_ari", "session_ari", "a CUDA GPU"); err != nil {
		t.Fatal(err)
	}
	for _, n := range hb.knownNodes(ctx) {
		if n.ID == ha.selfID() {
			hb.statusPull(ctx, n)
		}
	}
	var list []Status
	if c := b.owner(t, "GET", "/api/v1/statuses", nil, &list); c != 200 {
		t.Fatalf("statuses: %d", c)
	}
	if len(list) != 1 || list[0].For != ha.selfID()+"::session_ari" {
		t.Fatalf("the pc should see the laptop's status, qualified: %+v", list)
	}
	board := hb.statusBoard(ctx, Session{ID: "session_bo"})
	if !strings.Contains(board, list[0].ID) || !strings.Contains(board, "a CUDA GPU") {
		t.Fatalf("an agent on the pc should read it on its board:\n%s", board)
	}
	if err := hb.statusAct(ctx, statusAction{Status: list[0].ID, Member: hb.qualify("session_bo"), Text: "RTX 4070"}); err != nil && !strings.Contains(err.Error(), "409") {
		t.Fatal(err)
	}
	home, _ := ha.statusGet(ctx, list[0].ID)
	if len(home.Offers) != 1 || home.Offers[0].Member != hb.selfID()+"::session_bo" || home.Offers[0].Room == "" {
		t.Fatalf("the offer should be recorded on the laptop with a side chat: %+v", home.Offers)
	}
	if err := hb.statusAct(ctx, statusAction{Status: list[0].ID, Member: hb.qualify("session_bo"), Close: true}); err == nil {
		t.Fatal("only the requester or poster may close a status")
	}
}
