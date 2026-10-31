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
	tm.set("bo", "reply.txt", "[approve] keypad built and checked")
	tm.set("ari", "reply.txt", "[done] all parts verified together")
	// The owner talks to a worker directly; its unaddressed reply goes to the lead.
	tm.say(t, room.ID, "@agy-2 build the keypad")
	run := settled(t, tm.s, room.ID)
	if run.Status != "completed" {
		t.Fatalf("the lead's reviewed [done] should complete the run: %+v", run)
	}
	got := tm.replies(t, room.ID)
	if got["bo"] != 2 || got["ari"] != 1 {
		t.Fatalf("the worker reports, the lead finishes, the worker approves; got %v", got)
	}
}

func TestTheLeadsDoneWaitsForTheDesignOwnersApproval(t *testing.T) {
	tm := newTeam(t, "ari", "bo", "cy")
	room := leadGroup(t, tm, "ari", "ari", "bo", "cy")
	call(t, tm.s, "POST", "/api/v1/rooms/"+room.ID+"/members/"+tm.agents["cy"].ID+"/role", map[string]string{"role": "Design owner"}, nil)
	tm.set("ari", "reply.txt", "[done] the page is finished")
	tm.set("bo", "reply.txt", "[approve] ship it")
	tm.set("cy", "reply.txt", "@lead the hero still uses a gradient blob; replace it, note")
	// Numbered, as real review notes differ from round to round.
	tm.set("ari", "count", "0")
	tm.set("cy", "count", "0")
	reviews := func() int {
		n := 0
		for _, m := range tm.history(t, room.ID) {
			if m.Sender == "mace" && strings.Contains(m.Text, "reports the task done") {
				n++
			}
		}
		return n
	}

	// The design owner keeps asking for changes: each [done] goes back to
	// review, until the round limit lets the lead's [done] stand.
	tm.say(t, room.ID, "@lead make the page")
	settled(t, tm.s, room.ID)
	got := tm.replies(t, room.ID)
	if n := reviews(); n != maxReviews || got["cy"] != maxReviews || got["ari"] != maxReviews+1 || got["bo"] != 0 {
		for _, m := range tm.history(t, room.ID) {
			t.Logf("%s hop=%d [%s] %s", m.Sender[:min(12, len(m.Sender))], m.Hop, m.Flair, m.Text[:min(70, len(m.Text))])
		}
		t.Fatalf("expected %d reviews by the design owner only; reviews %d, replies %v", maxReviews, n, got)
	}

	// The owner writes again; this time the design owner approves at once.
	tm.set("cy", "reply.txt", "[approve] the hero is fixed and it reads well")
	tm.say(t, room.ID, "@lead fix it")
	run := settled(t, tm.s, room.ID)
	if n := reviews(); n != maxReviews+1 || run.Status != "completed" {
		t.Fatalf("one more review should finish the task: reviews %d, run %+v", n, run)
	}
	last := tm.history(t, room.ID)
	if final := last[len(last)-1]; final.Sender != tm.agents["cy"].ID || final.Flair != "approve" {
		t.Fatalf("the task should end on the design owner's approval: %+v", final)
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

func TestABriefToTheLeadWakesOnlyTheLead(t *testing.T) {
	tm := newTeam(t, "ari", "bo")
	room := leadGroup(t, tm, "ari", "ari", "bo")
	tm.set("ari", "reply.txt", "[PASS]")
	tm.say(t, room.ID, "@lead plan this, @agy-2 will help you")
	settled(t, tm.s, room.ID)
	if b, _ := os.ReadFile(tm.timeline); strings.Contains(string(b), "start bo") {
		t.Fatal("coworkers named in a brief to the lead wait for the lead's plan")
	}
}
