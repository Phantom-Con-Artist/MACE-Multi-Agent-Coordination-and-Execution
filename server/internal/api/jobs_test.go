package api

import (
	"context"
	"strings"
	"testing"
)

func TestNeedsAreCheckedAgainstMeasuredFacts(t *testing.T) {
	f := MachineFacts{Name: "pc", OS: "cachyos", Arch: "x86_64", Cores: 4, MemoryGB: 32, Tools: []string{"go", "pacman"},
		GPUs: []GPU{{Vendor: "nvidia", Model: "RTX 4070", CUDA: "12.4"}}}
	if miss := needsMet(f, []string{"go", "cuda", "gpu", "cachyos", "x86_64", "ram>=16", "on:pc"}); len(miss) != 0 {
		t.Fatalf("the PC has all of these: %v", miss)
	}
	if miss := needsMet(f, []string{"docker", "arm64", "cores>=8"}); len(miss) != 3 {
		t.Fatalf("the PC lacks docker, arm64, and 8 cores: %v", miss)
	}
}

func jobsOf(t *testing.T, tm *team) []Job {
	t.Helper()
	var list []Job
	call(t, tm.s, "GET", "/api/v1/jobs", nil, &list)
	return list
}

func TestJobsAreClaimedByMachinesThatFitAndApprovedByTheLead(t *testing.T) {
	tm := newTeam(t, "ari", "bo")
	room := tm.group(t, "ari", "bo")
	if c := call(t, tm.s, "POST", "/api/v1/chats/"+room.ID+"/policy", Policy{Mode: "team", Lead: tm.agents["ari"].ID}, nil); c != 200 {
		t.Fatalf("policy: %d", c)
	}
	tm.set("ari", "reply.txt", "[plan] two parts [job: build the site | needs: go] [job: train the model | needs: no-such-tool]")
	tm.set("bo", "reply.txt", "[PASS]")
	tm.say(t, room.ID, "@agy plan it")
	settled(t, tm.s, room.ID)
	list := jobsOf(t, tm)
	if len(list) != 2 {
		t.Fatalf("the lead's two jobs should be posted: %+v", list)
	}
	site, train := list[0], list[1]
	if !strings.Contains(site.Title, "site") {
		site, train = train, site
	}
	tm.set("bo", "reply.txt", "[claim "+train.ID+"] [claim "+site.ID+"] on it")
	tm.say(t, room.ID, "@agy-2 take what fits")
	settled(t, tm.s, room.ID)
	if p := tm.promptsOf(t, "bo"); !strings.Contains(p, "MACHINES (measured by MACE)") || !strings.Contains(p, site.ID) || strings.Contains(p, "OPEN JOBS YOUR MACHINE CAN TAKE (claim one with [claim <id>] only if you will do it now):\n- "+train.ID) {
		t.Fatalf("bo should see the machines and only the job its machine fits:\n%s", p)
	}
	byID := map[string]Job{}
	for _, j := range jobsOf(t, tm) {
		byID[j.ID] = j
	}
	if byID[train.ID].Owner != "" || byID[site.ID].Owner != tm.s.handler.qualify(tm.agents["bo"].ID) {
		t.Fatalf("bo gets the job its machine fits and not the other: %+v", byID)
	}
	refused := false
	for _, m := range tm.history(t, room.ID) {
		refused = refused || strings.Contains(m.Text, "needs no-such-tool")
	}
	if !refused {
		t.Fatal("the refused claim should say what is missing")
	}

	tm.set("bo", "reply.txt", "[done "+site.ID+"] site builds, go test ./... passes")
	tm.set("ari", "reply.txt", "[approve "+site.ID+"] looks right")
	tm.say(t, room.ID, "@agy-2 finish up")
	settled(t, tm.s, room.ID)
	if p := tm.promptsOf(t, "ari"); !strings.Contains(p, "JOBS YOU LEAD") {
		t.Fatalf("the lead should see the jobs it leads:\n%s", p)
	}
	tm.say(t, room.ID, "@agy approve it if it is right")
	settled(t, tm.s, room.ID)
	for _, j := range jobsOf(t, tm) {
		if j.ID == site.ID && j.State != "approved" {
			t.Fatalf("the lead's approval should finish the job: %+v", j)
		}
	}
}

func TestAgentsNearTheirLimitsAreToldToHandOff(t *testing.T) {
	tm := newTeam(t, "ari")
	ctx := context.Background()
	h := tm.s.handler
	used := 91.0
	h.noteHealth(ctx, tm.agents["ari"].ID, &Usage{Input: 900_000, WeeklyUsed: &used}, nil)
	line := h.healthLine(ctx, tm.agents["ari"])
	if !strings.Contains(line, "weekly quota 91% used") || !strings.Contains(line, "[pass <id> @handle]") {
		t.Fatalf("a nearly spent agent should be told to hand off: %q", line)
	}
	h.noteHealth(ctx, tm.agents["ari"].ID, nil, errString("Error 429: Too Many Requests, quota exceeded"))
	if w := h.healthWarnings(ctx, tm.agents["ari"].ID); !strings.Contains(strings.Join(w, ";"), "reported a limit") {
		t.Fatalf("a CLI quota error should count: %v", w)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
