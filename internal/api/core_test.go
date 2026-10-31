package api

import (
	"os"
	"strings"
	"testing"
	"time"
)

// The owner's word wins: a new owner message in a group stops the turns of the
// agents it addresses, and replies started before it are never posted.
func TestOwnerMessageSupersedesWorkInProgress(t *testing.T) {
	tm := newTeam(t, "ari", "bo")
	room := tm.group(t, "ari", "bo")
	tm.set("ari", "delay", "1.5")
	tm.set("bo", "delay", "1.5")
	tm.say(t, room.ID, "talk about lunch")
	waitFor(t, "both started", func() bool {
		b, _ := os.ReadFile(tm.timeline)
		return strings.Count(string(b), "start ") == 2
	})
	tm.set("ari", "delay", "0")
	tm.set("bo", "delay", "0")
	tm.set("ari", "reply.txt", "on the new task")
	tm.set("bo", "reply.txt", "on the new task")
	tm.say(t, room.ID, "stop, new task: check the build")
	settled(t, tm.s, room.ID)
	for _, m := range tm.history(t, room.ID) {
		if m.Sender != "you" && m.Sender != "mace" && !strings.Contains(m.Text, "new task") {
			t.Fatalf("a reply started before the owner's new message was posted: %q", m.Text)
		}
	}
	if got := tm.replies(t, room.ID); got["ari"] != 1 || got["bo"] != 1 {
		t.Fatalf("each agent should answer the new message once, got %v", got)
	}
}

func TestUnaddressedAgentsKeepWorking(t *testing.T) {
	tm := newTeam(t, "ari", "bo")
	room := tm.group(t, "ari", "bo")
	tm.set("bo", "delay", "1")
	tm.say(t, room.ID, "@bo run the long check")
	waitFor(t, "bo started", func() bool { b, _ := os.ReadFile(tm.timeline); return strings.Contains(string(b), "start bo") })
	tm.say(t, room.ID, "@ari quick question")
	settled(t, tm.s, room.ID)
	if got := tm.replies(t, room.ID); got["bo"] != 1 || got["ari"] != 1 {
		t.Fatalf("bo's work should finish while ari answers, got %v", got)
	}
}

func TestAgentsNeverStartWithoutTheOwner(t *testing.T) {
	t.Setenv("MACE_CONTINUOUS_GAP_SECONDS", "0")
	tm := newTeam(t, "ari", "bo")
	room := tm.group(t, "ari", "bo")
	call(t, tm.s, "POST", "/api/v1/chats/"+room.ID+"/policy", Policy{Mode: "continuous"}, nil)
	if code := call(t, tm.s, "POST", "/api/v1/chats/"+room.ID+"/start", map[string]any{}, nil); code < 400 {
		t.Fatalf("starting a group without an owner message must be refused, got %d", code)
	}
	time.Sleep(200 * time.Millisecond)
	if h := tm.history(t, room.ID); len(h) != 0 {
		t.Fatalf("nobody should speak before the owner: %+v", h)
	}
}

func TestUntilDoneEndsWhenAnAgentReportsDone(t *testing.T) {
	t.Setenv("MACE_CONTINUOUS_GAP_SECONDS", "0")
	tm := newTeam(t, "ari", "bo")
	room := tm.group(t, "ari", "bo")
	call(t, tm.s, "POST", "/api/v1/chats/"+room.ID+"/policy", Policy{Mode: "continuous"}, nil)
	tm.set("ari", "reply.txt", "[done] build is green")
	tm.set("bo", "reply.txt", "[PASS]")
	tm.say(t, room.ID, "@ari check the build")
	run := settled(t, tm.s, room.ID)
	if run.Status != "completed" {
		t.Fatalf("the run should complete on [done], got %+v", run)
	}
	h := tm.history(t, room.ID)
	var done *Message
	for i := range h {
		if h[i].Sender == tm.agents["ari"].ID {
			done = &h[i]
		}
	}
	if done == nil || done.Flair != "done" || strings.Contains(done.Text, "[done]") {
		t.Fatalf("the [done] tag should become the message's flair: %+v", done)
	}
}
