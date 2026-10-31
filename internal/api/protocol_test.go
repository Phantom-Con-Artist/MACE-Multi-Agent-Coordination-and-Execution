package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// scriptedAgy is a fake agy that speaks the real stream-json format. Each agent
// works in its own folder, which holds its reply text, an optional delay, a
// counter appended to the reply when "count" exists, and a log of prompts.
const scriptedAgy = `#!/bin/sh
me=$(basename "$PWD")
echo "start $me" >> "$TIMELINE"
sleep "$(cat delay 2>/dev/null || echo 0)"
reply=$(cat reply.txt)
if [ -e count ]; then n=$(( $(cat count) + 1 )); echo $n > count; reply="$reply $n"; fi
printf '%s\n' "$@" >> prompts.log; echo ---- >> prompts.log
env | grep '^MACE_' > env.log
echo '{"event":"init","conversation_id":"conv-'$me'"}'
echo '{"event":"step_update","step_update":{"step_index":1,"state":"DONE","step_type":"tool","tool_name":"read_file","tool_info":{"name":"read_file","parameters":{"Path":"/w/plan.md"}}}}'
printf '{"event":"result","result":{"conversation_id":"conv-%s","status":"SUCCESS","response":"%s"}}\n' "$me" "$reply"
echo "end $me" >> "$TIMELINE"
`

type team struct {
	s        *Server
	timeline string
	agents   map[string]Session
	dirs     map[string]string
}

func newTeam(t *testing.T, names ...string) *team {
	t.Helper()
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	os.WriteFile(filepath.Join(bin, "agy"), []byte(scriptedAgy), 0700)
	tm := &team{timeline: filepath.Join(bin, "timeline"), agents: map[string]Session{}, dirs: map[string]string{}}
	t.Setenv("TIMELINE", tm.timeline)
	s, sup, store := setupTestServer(t, "secret")
	t.Cleanup(func() { sup.Close(); store.Close() })
	tm.s = s
	root := t.TempDir()
	for _, name := range names {
		dir := filepath.Join(root, name)
		os.MkdirAll(dir, 0700)
		os.WriteFile(filepath.Join(dir, "reply.txt"), []byte("reply from "+name), 0600)
		var a Session
		if call(t, s, "POST", "/api/v1/sessions", map[string]string{"provider": "agy", "working_dir": dir}, &a) != 201 {
			t.Fatal("create agent")
		}
		tm.agents[name], tm.dirs[name] = a, dir
	}
	return tm
}

func (tm *team) set(name, file, value string) {
	os.WriteFile(filepath.Join(tm.dirs[name], file), []byte(value), 0600)
}

func (tm *team) group(t *testing.T, names ...string) Room {
	t.Helper()
	ids := []string{}
	for _, n := range names {
		ids = append(ids, tm.agents[n].ID)
	}
	var room Room
	if call(t, tm.s, "POST", "/api/v1/rooms", map[string]any{"name": "Team", "members": ids}, &room) != 201 {
		t.Fatal("create group")
	}
	return room
}

func (tm *team) say(t *testing.T, room, text string) Message {
	t.Helper()
	var m Message
	if code := call(t, tm.s, "POST", "/api/v1/chats/"+room+"/messages", map[string]string{"text": text}, &m); code != 201 {
		t.Fatalf("send failed: %d", code)
	}
	return m
}

func (tm *team) history(t *testing.T, room string) []Message {
	t.Helper()
	var out []Message
	call(t, tm.s, "GET", "/api/v1/chats/"+room+"/messages", nil, &out)
	return out
}

func (tm *team) replies(t *testing.T, room string) map[string]int {
	out := map[string]int{}
	for _, m := range tm.history(t, room) {
		for name, a := range tm.agents {
			if m.Sender == a.ID {
				out[name]++
			}
		}
	}
	return out
}

func (tm *team) prompts(name string) []string {
	b, _ := os.ReadFile(filepath.Join(tm.dirs[name], "prompts.log"))
	return strings.Split(strings.TrimSuffix(string(b), "----\n"), "----\n")
}

func settled(t *testing.T, s *Server, room string) Run {
	t.Helper()
	return waitRun(t, s, room, func(r Run) bool { return r.Status != "running" && r.Status != "idle" })
}

func TestOwnerMessageWakesEveryoneInParallel(t *testing.T) {
	tm := newTeam(t, "ari", "bo")
	tm.set("ari", "delay", "0.6")
	tm.set("bo", "delay", "0.6")
	room := tm.group(t, "ari", "bo")
	tm.say(t, room.ID, "Plan the release")
	waitRun(t, tm.s, room.ID, func(r Run) bool { return len(r.ActiveMembers) == 2 })
	run := settled(t, tm.s, room.ID)
	if run.Status != "completed" || run.Replies != 2 {
		t.Fatalf("both agents should answer once: %+v", run)
	}
	b, _ := os.ReadFile(tm.timeline)
	lines := strings.Fields(strings.ReplaceAll(string(b), "\n", " "))
	if len(lines) < 4 || lines[0] != "start" || lines[2] != "start" {
		t.Fatalf("both agents must start before either finishes, timeline: %q", b)
	}
	for _, m := range tm.history(t, room.ID) {
		if m.Sender == tm.agents["ari"].ID && (len(m.Activity) != 1 || m.Activity[0].Title != "Read `plan.md`") {
			t.Fatalf("a reply should keep the tool steps it took: %+v", m.Activity)
		}
	}
}

func TestMentionsAddressOnlyThoseAgents(t *testing.T) {
	tm := newTeam(t, "ari", "bo", "cy")
	room := tm.group(t, "ari", "bo", "cy")
	m := tm.say(t, room.ID, "@agy-2 please check the build")
	if len(m.Mentions) != 1 || m.Mentions[0] != tm.agents["bo"].ID {
		t.Fatalf("mention not resolved on the server: %+v", m.Mentions)
	}
	settled(t, tm.s, room.ID)
	if got := tm.replies(t, room.ID); got["bo"] != 1 || got["ari"] != 0 || got["cy"] != 0 {
		t.Fatalf("only @agy-2 should answer: %v", got)
	}
	tm.say(t, room.ID, "@all quick check-in")
	settled(t, tm.s, room.ID)
	if got := tm.replies(t, room.ID); got["ari"] != 1 || got["bo"] != 2 || got["cy"] != 1 {
		t.Fatalf("@all should wake everyone: %v", got)
	}
}

func TestAgentMentionHandsOffAndPassIsSilent(t *testing.T) {
	tm := newTeam(t, "ari", "bo")
	room := tm.group(t, "ari", "bo")
	tm.set("ari", "reply.txt", "@agy-2 can you verify this")
	tm.set("bo", "reply.txt", "[PASS]")
	call(t, tm.s, "POST", "/api/v1/chats/"+room.ID+"/policy", Policy{Mode: "reply", Limit: 10}, nil)
	tm.say(t, room.ID, "@agy draft the notes")
	run := settled(t, tm.s, room.ID)
	got := tm.replies(t, room.ID)
	if got["ari"] != 1 || got["bo"] != 0 || run.Status != "completed" {
		t.Fatalf("bo should take the handed-off turn and pass silently: %v %+v", got, run)
	}
	if len(tm.prompts("bo")) != 1 {
		t.Fatal("bo never got the handed-off turn")
	}
}

func TestHopLimitStopsEndlessHandoffs(t *testing.T) {
	tm := newTeam(t, "ari", "bo")
	room := tm.group(t, "ari", "bo")
	call(t, tm.s, "POST", "/api/v1/chats/"+room.ID+"/policy", Policy{Mode: "reply", Limit: 100}, nil)
	tm.set("ari", "reply.txt", "@agy-2 over to you for review number")
	tm.set("bo", "reply.txt", "@agy back to you with notes number")
	tm.set("ari", "count", "0")
	tm.set("bo", "count", "0")
	tm.say(t, room.ID, "@agy start")
	settled(t, tm.s, room.ID)
	got := tm.replies(t, room.ID)
	if got["ari"]+got["bo"] != hopLimit {
		t.Fatalf("expected the chain to stop after %d agent messages, got %v", hopLimit, got)
	}
	last := tm.history(t, room.ID)
	if note := last[len(last)-1]; note.Sender != "mace" || !strings.Contains(note.Text, "stopped the chain") {
		t.Fatalf("the stop must be visible, got %+v", note)
	}
}

func TestRepeatingAgentsArePaused(t *testing.T) {
	t.Setenv("MACE_CONTINUOUS_GAP_SECONDS", "0")
	tm := newTeam(t, "ari", "bo")
	room := tm.group(t, "ari", "bo")
	tm.set("ari", "reply.txt", "I agree, sounds good to me")
	tm.set("bo", "reply.txt", "I agree, sounds good to me")
	call(t, tm.s, "POST", "/api/v1/chats/"+room.ID+"/policy", Policy{Mode: "continuous"}, nil)
	tm.say(t, room.ID, "what do you think?")
	run := settled(t, tm.s, room.ID)
	if run.Status != "paused" {
		t.Fatalf("a repeating conversation should pause, got %+v", run)
	}
	h := tm.history(t, room.ID)
	if !strings.Contains(h[len(h)-1].Text, "kept repeating") {
		t.Fatal("the pause reason must be visible")
	}
}

// The owner can always write. A busy agent reads the new message on its next turn.
func TestOwnerCanWriteWhileAgentsWork(t *testing.T) {
	tm := newTeam(t, "ari")
	tm.set("ari", "delay", "0.5")
	id := tm.agents["ari"].ID
	tm.say(t, id, "first task")
	// Wait until the agent has its first prompt, so the second message is new to it.
	waitFor(t, "first turn started", func() bool { b, _ := os.ReadFile(tm.timeline); return strings.Contains(string(b), "start ari") })
	if m := tm.say(t, id, "also add tests"); m.Delivery["conversation"] != "delivered" {
		t.Fatalf("a message during work must be accepted: %+v", m.Delivery)
	}
	waitFor(t, "second turn", func() bool { return len(tm.prompts("ari")) == 2 })
	settled(t, tm.s, id)
	prompts := tm.prompts("ari")
	if !strings.Contains(prompts[1], "also add tests") || strings.Contains(prompts[1], "first task") {
		t.Fatalf("the second turn should be a delta with only the new message:\n%s", prompts[1])
	}
	if !strings.Contains(prompts[1], "--conversation") {
		t.Fatal("the second turn should resume the native session")
	}
}

// One coworker, one memory: the private chat and a group share a native session,
// never run at the same time, and each knows about the other.
func TestPrivateChatAndGroupShareOneCoworker(t *testing.T) {
	tm := newTeam(t, "ari", "bo")
	tm.set("ari", "delay", "0.4")
	room := tm.group(t, "ari", "bo")
	ari := tm.agents["ari"].ID
	tm.say(t, ari, "privately: what do you think of the plan")
	tm.say(t, room.ID, "@agy share your view with the team")
	settled(t, tm.s, ari)
	settled(t, tm.s, room.ID)
	b, _ := os.ReadFile(tm.timeline)
	events := []string{}
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if strings.HasSuffix(l, " ari") {
			events = append(events, strings.Fields(l)[0])
		}
	}
	if strings.Join(events, ",") != "start,end,start,end" {
		t.Fatalf("the same agent must never run two turns at once: %v", events)
	}
	prompts := tm.prompts("ari")
	if len(prompts) != 2 || !strings.Contains(prompts[1], "--conversation\nconv-ari") {
		t.Fatal("both conversations must use the same native session")
	}
	if !strings.Contains(prompts[0], "Private chat with the owner") || !strings.Contains(prompts[0], `Group "Team"`) {
		t.Fatal("the session should learn about both conversations")
	}
	// The resumed session already holds that list, so only who it is comes again.
	if !strings.Contains(prompts[1], "MACE AWARENESS") || strings.Contains(prompts[1], `Group "Team"`) {
		t.Fatal("a resumed turn should not resend an unchanged conversation list")
	}
	if !strings.Contains(prompts[1], "THIS TURN IS FOR") {
		t.Fatal("the turn must say which conversation it is for")
	}
}

func TestMessagePagingOverAPI(t *testing.T) {
	tm := newTeam(t, "ari")
	room := tm.agents["ari"].ID
	for i := 0; i < 5; i++ {
		tm.s.handler.systemNote(room, "note")
	}
	r := httptest.NewRequest("GET", "/api/v1/chats/"+room+"/messages?limit=3", nil)
	r.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	tm.s.Router().ServeHTTP(w, r)
	var page []Message
	json.Unmarshal(w.Body.Bytes(), &page)
	if len(page) != 3 || w.Header().Get("X-More") != "true" || page[0].Seq >= page[2].Seq {
		t.Fatalf("newest page wrong: %d more=%s", len(page), w.Header().Get("X-More"))
	}
	var older []Message
	call(t, tm.s, "GET", "/api/v1/chats/"+room+"/messages?limit=3&before="+itoa64(page[0].Seq), nil, &older)
	if len(older) != 2 || older[1].Seq >= page[0].Seq {
		t.Fatal("older page wrong")
	}
}

func itoa64(n int64) string { b, _ := json.Marshal(n); return string(b) }

func readEvents(t *testing.T, body *bufio.Reader, want int) []string {
	t.Helper()
	out := []string{}
	for len(out) < want {
		line, err := body.ReadString('\n')
		if err != nil {
			t.Fatalf("stream ended: %v (got %v)", err, out)
		}
		if strings.HasPrefix(line, "id: ") || strings.HasPrefix(line, "event: ") {
			out = append(out, strings.TrimSpace(line))
		}
	}
	return out
}

func TestEventStreamDeliversAndResumes(t *testing.T) {
	tm := newTeam(t, "ari")
	srv := httptest.NewServer(tm.s.Router())
	defer srv.Close()
	open := func(last string) (*http.Response, *bufio.Reader) {
		req, _ := http.NewRequest("GET", srv.URL+"/api/v1/stream", nil)
		req.Header.Set("Authorization", "Bearer secret")
		if last != "" {
			req.Header.Set("Last-Event-ID", last)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res, bufio.NewReader(res.Body)
	}
	res, body := open("")
	got := readEvents(t, body, 2)
	if got[1] != "event: reset" {
		t.Fatalf("a fresh client starts with a reset, got %v", got)
	}
	room := tm.agents["ari"].ID
	tm.s.handler.systemNote(room, "hello")
	got = readEvents(t, body, 2)
	if got[1] != "event: message" {
		t.Fatalf("expected a message event, got %v", got)
	}
	lastID := strings.TrimPrefix(got[0], "id: ")
	res.Body.Close()

	tm.s.handler.systemNote(room, "while away")
	res, body = open(lastID)
	got = readEvents(t, body, 2)
	if got[1] != "event: message" {
		t.Fatalf("a reconnecting client should receive what it missed, got %v", got)
	}
	res.Body.Close()
	res, body = open("old-epoch-5")
	if got = readEvents(t, body, 2); got[1] != "event: reset" {
		t.Fatalf("an unknown event ID must force a reload, got %v", got)
	}
	res.Body.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = ctx
}

func TestInboxAndReceipts(t *testing.T) {
	tm := newTeam(t, "ari")
	id := tm.agents["ari"].ID
	m := tm.say(t, id, "hello")
	settled(t, tm.s, id)
	var inbox []struct {
		Room   string  `json:"room"`
		Latest Message `json:"latest"`
		Unread int     `json:"unread"`
	}
	call(t, tm.s, "POST", "/api/v1/inbox", map[string]any{"seen": map[string]int64{id: m.Seq}}, &inbox)
	if len(inbox) != 1 || inbox[0].Room != id || inbox[0].Latest.Sender != id || inbox[0].Unread != 1 {
		t.Fatalf("inbox should show the agent's reply as one unread message: %+v", inbox)
	}
	var receipts map[string]int64
	call(t, tm.s, "GET", "/api/v1/chats/"+id+"/receipts", nil, &receipts)
	if receipts[id] < m.Seq {
		t.Fatalf("the agent read the owner's message, so its receipt must cover it: %v", receipts)
	}
}
