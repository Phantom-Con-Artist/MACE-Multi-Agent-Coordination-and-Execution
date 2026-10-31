package api

import (
	"os"
	"strings"
	"testing"
)

func leadGroup(t *testing.T, tm *team, lead string, names ...string) Room {
	t.Helper()
	room := tm.group(t, names...)
	if c := call(t, tm.s, "POST", "/api/v1/chats/"+room.ID+"/policy", Policy{Mode: "team", Lead: tm.agents[lead].ID}, nil); c != 200 {
		t.Fatalf("team policy: %d", c)
	}
	return room
}

func TestOwnerMessagesGoToTheLead(t *testing.T) {
	tm := newTeam(t, "ari", "bo")
	room := leadGroup(t, tm, "ari", "ari", "bo")
	tm.set("ari", "reply.txt", "[PASS]")
	tm.say(t, room.ID, "hi team, build a calculator")
	settled(t, tm.s, room.ID)
	if b, _ := os.ReadFile(tm.timeline); strings.Contains(string(b), "start bo") {
		t.Fatal("only the lead should receive an unaddressed owner message")
	}
	if !strings.Contains(header(t, tm, room, "ari"), "YOU ARE THE LEAD") || !strings.Contains(header(t, tm, room, "bo"), "is the lead") {
		t.Fatal("the lead and the workers should be told who leads")
	}
}

func TestWorkersReportToTheLeadAndOnlyTheLeadFinishes(t *testing.T) {
	tm := newTeam(t, "ari", "bo")
	room := leadGroup(t, tm, "ari", "ari", "bo")
	tm.set("bo", "reply.txt", "[done] my part is built")
	tm.set("ari", "reply.txt", "[done] all parts verified together")
	// The owner talks to a worker directly; its unaddressed reply goes to the lead.
	tm.say(t, room.ID, "@agy-2 build the keypad")
	run := settled(t, tm.s, room.ID)
	if run.Status != "completed" {
		t.Fatalf("the lead's [done] should complete the run: %+v", run)
	}
	got := tm.replies(t, room.ID)
	if got["bo"] != 1 || got["ari"] != 1 {
		t.Fatalf("the worker reports once and the lead finishes once, got %v", got)
	}
}

func TestAtLeadReachesTheLead(t *testing.T) {
	tm := newTeam(t, "ari", "bo")
	room := leadGroup(t, tm, "ari", "ari", "bo")
	tm.set("bo", "reply.txt", "@lead should the display show history?")
	tm.set("ari", "reply.txt", "[PASS]")
	tm.say(t, room.ID, "@agy-2 start on the display")
	settled(t, tm.s, room.ID)
	for _, m := range tm.history(t, room.ID) {
		if m.Sender == tm.agents["bo"].ID && (len(m.Mentions) != 1 || m.Mentions[0] != tm.agents["ari"].ID) {
			t.Fatalf("@lead should mention the lead: %+v", m.Mentions)
		}
	}
	if b, _ := os.ReadFile(tm.timeline); strings.Count(string(b), "start agy") == 0 && !strings.Contains(string(b), "start ari") {
		t.Fatal("the lead should get the worker's question")
	}
}

func TestTheLeadMustBeAMember(t *testing.T) {
	tm := newTeam(t, "ari", "bo", "cy")
	room := tm.group(t, "ari", "bo")
	if c := call(t, tm.s, "POST", "/api/v1/chats/"+room.ID+"/policy", Policy{Mode: "team", Lead: tm.agents["cy"].ID}, nil); c != 400 {
		t.Fatalf("a lead outside the group must be refused, got %d", c)
	}
	if c := call(t, tm.s, "POST", "/api/v1/chats/"+room.ID+"/policy", Policy{Mode: "team"}, nil); c != 400 {
		t.Fatalf("a goal group needs a lead, got %d", c)
	}
}

func TestLeadBriefsThenDividesByCapability(t *testing.T) {
	tm := newTeam(t, "ari", "bo")
	room := leadGroup(t, tm, "ari", "ari", "bo")
	lead, worker := header(t, tm, room, "ari"), header(t, tm, room, "bo")
	for _, want := range []string{"Brief:", "Divide:", "[plan]", "Runs Antigravity"} {
		if !strings.Contains(lead, want) {
			t.Errorf("the lead's turn is missing %q:\n%s", want, lead)
		}
	}
	if !strings.Contains(worker, "until the lead assigns you a part") {
		t.Errorf("workers should wait for an assignment:\n%s", worker)
	}
}
