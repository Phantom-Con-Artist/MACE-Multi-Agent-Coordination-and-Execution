package api

import (
	"context"
	"strings"
	"testing"
)

func header(t *testing.T, tm *team, room Room, member string) string {
	t.Helper()
	var g Room
	tm.s.handler.store.Document(context.Background(), "room", room.ID, &g)
	p := tm.s.handler.policyFor(context.Background(), room.ID, g.Members)
	return tm.s.handler.conversationHeader(context.Background(), room.ID, tm.agents[member].ID, g.Members, &g, p)
}

func TestGroupTurnsCarryTheRequestStyleAndFlairs(t *testing.T) {
	tm := newTeam(t, "ari", "bo")
	room := tm.group(t, "ari", "bo")
	tm.set("ari", "reply.txt", "[PASS]")
	tm.set("bo", "reply.txt", "[PASS]")
	tm.say(t, room.ID, "ship the login page by tonight")
	settled(t, tm.s, room.ID)
	h := header(t, tm, room, "ari")
	for _, want := range []string{"ship the login page by tonight", "two short lines", "ik", "[question]", "[role:"} {
		if !strings.Contains(h, want) {
			t.Errorf("group header is missing %q:\n%s", want, h)
		}
	}
	call(t, tm.s, "POST", "/api/v1/chats/"+room.ID+"/policy", Policy{Mode: "reply", Limit: 2, Tone: "professional"}, nil)
	if h := header(t, tm, room, "ari"); strings.Contains(h, "lmao") || !strings.Contains(h, "professional") {
		t.Fatalf("a professional group should not invite slang:\n%s", h)
	}
}

func TestAgentsChooseAndChangeTheirOwnRoles(t *testing.T) {
	tm := newTeam(t, "ari", "bo")
	room := tm.group(t, "ari", "bo")
	tm.set("ari", "reply.txt", "[role: owns the API tests] [plan] ill write the tests, @bo take the UI?")
	tm.set("bo", "reply.txt", "[PASS]")
	tm.say(t, room.ID, "@ari build the login feature with bo")
	settled(t, tm.s, room.ID)

	var ari *Message
	for _, m := range tm.history(t, room.ID) {
		if m.Sender == tm.agents["ari"].ID {
			ari = &m
		}
	}
	if ari == nil || ari.Flair != "plan" || ari.Role != "owns the API tests" || strings.HasPrefix(ari.Text, "[") {
		t.Fatalf("the role claim and flair should be taken off the message: %+v", ari)
	}
	if h := header(t, tm, room, "bo"); !strings.Contains(h, "owns the API tests") || !strings.Contains(strings.ToLower(h), "self-chosen") {
		t.Fatalf("bo should see ari's self-chosen role:\n%s", h)
	}
	// An owner-assigned role stays authoritative and is shown first.
	call(t, tm.s, "POST", "/api/v1/rooms/"+room.ID+"/members/"+tm.agents["ari"].ID+"/role", map[string]string{"role": "Security reviewer"}, nil)
	h := header(t, tm, room, "bo")
	if !strings.Contains(h, "Owner-assigned role: Security reviewer") || !strings.Contains(h, "owns the API tests") {
		t.Fatalf("both roles should show, owner's first:\n%s", h)
	}
}

func TestLongRepliesGetANudge(t *testing.T) {
	tm := newTeam(t, "ari", "bo")
	room := tm.group(t, "ari", "bo")
	tm.set("ari", "reply.txt", strings.Repeat("this is a very long routine reply ", 30))
	tm.set("bo", "reply.txt", "[PASS]")
	tm.say(t, room.ID, "@ari thoughts?")
	settled(t, tm.s, room.ID)
	if h := header(t, tm, room, "ari"); !strings.Contains(h, "Your last message here was") {
		t.Fatalf("a long routine reply should be followed by a reminder:\n%s", h)
	}
	if h := header(t, tm, room, "bo"); strings.Contains(h, "Your last message here was") {
		t.Fatal("the reminder is only for the agent that wrote long")
	}
}

func TestHandlesQuotedInCodeAreNotMentions(t *testing.T) {
	handles := map[string]string{"a": "agy", "m": "muse"}
	got, _ := parseMentions("verified group targeting (`@agy`) and\n```\n@muse here\n```\nthanks @muse", handles)
	if len(got) != 1 || got[0] != "m" {
		t.Fatalf("only the plain @muse is a mention, got %v", got)
	}
}
