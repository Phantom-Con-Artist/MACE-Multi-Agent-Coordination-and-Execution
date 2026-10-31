package api

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (tm *team) promptsOf(t *testing.T, name string) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(tm.dirs[name], "prompts.log"))
	return string(b)
}

func TestSharedFilesReachOnlyTheTaggedAgent(t *testing.T) {
	tm := newTeam(t, "ari", "bo", "cy")
	room := tm.group(t, "ari", "bo", "cy")
	os.WriteFile(filepath.Join(tm.dirs["ari"], "notes.md"), []byte("secret plan"), 0600)
	tm.set("ari", "reply.txt", "here you go @agy-2 [share: notes.md]")
	tm.set("bo", "reply.txt", "[PASS]")
	tm.set("cy", "reply.txt", "[PASS]")
	tm.say(t, room.ID, "@ari send bo the notes")
	settled(t, tm.s, room.ID)

	var shared *Message
	for _, m := range tm.history(t, room.ID) {
		if m.Sender == tm.agents["ari"].ID {
			shared = &m
		}
	}
	want := strings.TrimPrefix(filepath.Join(tm.dirs["ari"], "notes.md"), "/")
	if shared == nil || shared.File != want || strings.Contains(shared.Text, "[share") {
		t.Fatalf("the file should be attached and the tag removed: %+v", shared)
	}
	// The next turns: bo is tagged and gets the path; cy only learns a file was shared.
	tm.set("bo", "reply.txt", "[PASS]")
	tm.say(t, room.ID, "everyone: anything else?")
	settled(t, tm.s, room.ID)
	if p := tm.promptsOf(t, "bo"); !strings.Contains(p, filepath.Join(tm.dirs["ari"], "notes.md")) {
		t.Fatal("the tagged agent should get the file's path")
	}
	p := tm.promptsOf(t, "cy")
	if strings.Contains(p, filepath.Join(tm.dirs["ari"], "notes.md")) || !strings.Contains(p, "shared notes.md with @") {
		t.Fatalf("an untagged agent should only see that a file was shared:\n%s", p)
	}
}

func TestLargePastesBecomeFiles(t *testing.T) {
	tm := newTeam(t, "ari")
	room := tm.group(t, "ari")
	block := "```go\n" + strings.Repeat("fmt.Println(\"x\")\n", 300) + "```"
	m := Message{Room: room.ID, Sender: tm.agents["ari"].ID, Text: "full file below\n" + block}
	files := tm.s.handler.attachShares(context.Background(), &m, tm.agents["ari"])
	if len(files) != 1 || strings.Contains(m.Text, "fmt.Println") || !strings.Contains(m.Text, "shared as") {
		t.Fatalf("a long code block should move into a file: %d files, text %q", len(files), m.Text)
	}
	b, err := os.ReadFile("/" + files[0])
	if err != nil || !strings.Contains(string(b), "fmt.Println") {
		t.Fatalf("the file should hold the code: %v", err)
	}
}

func TestSideChatsAreVisibleToTheOwnerAndReportBack(t *testing.T) {
	tm := newTeam(t, "ari", "bo")
	room := tm.group(t, "ari", "bo")
	tm.set("ari", "reply.txt", "[dm @agy-2] can you share your db creds file privately?")
	tm.set("bo", "reply.txt", "[PASS]")
	tm.say(t, room.ID, "@ari sort out the db access with bo")
	settled(t, tm.s, room.ID)

	var rooms []Room
	call(t, tm.s, "GET", "/api/v1/rooms", nil, &rooms)
	var side *Room
	for i := range rooms {
		if rooms[i].Parent == room.ID {
			side = &rooms[i]
		}
	}
	if side == nil || len(side.Members) != 2 {
		t.Fatalf("a side chat should exist under the group: %+v", rooms)
	}
	for _, m := range tm.history(t, room.ID) {
		if strings.Contains(m.Text, "db creds") {
			t.Fatal("the private message must not appear in the group")
		}
	}
	h := tm.history(t, side.ID)
	if len(h) == 0 || !strings.Contains(h[0].Text, "db creds") {
		t.Fatalf("the side chat should hold the message: %+v", h)
	}
	// bo answers in the side chat; a [group] reply goes back to the group.
	settled(t, tm.s, side.ID)
	tm.set("bo", "reply.txt", "[group] sorted, access works now")
	tm.say(t, side.ID, "@agy-2 wrap up and tell the group")
	settled(t, tm.s, side.ID)
	found := false
	for _, m := range tm.history(t, room.ID) {
		found = found || strings.Contains(m.Text, "access works now")
	}
	if !found {
		t.Fatal("a [group] reply in a side chat should be posted to the group")
	}
}
