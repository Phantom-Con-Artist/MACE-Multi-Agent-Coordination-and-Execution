package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Usage is what one turn, or a sum of turns, cost in tokens, as the agent's
// CLI reported it. Agents on other machines report theirs with the turn's
// result, so the hub records what every machine spends.
type Usage struct {
	Input     int64 `json:"input"`
	Cached    int64 `json:"cached,omitempty"`
	Output    int64 `json:"output"`
	Reasoning int64 `json:"reasoning,omitempty"`
	Calls     int   `json:"calls,omitempty"`
	Turns     int   `json:"turns,omitempty"`
	// WeeklyUsed is the account's own share of its weekly allowance, in
	// percent, when the CLI reports one (Muse does).
	WeeklyUsed *float64 `json:"weekly_used,omitempty"`
}

func (u *Usage) add(o Usage) {
	u.Input += o.Input
	u.Cached += o.Cached
	u.Output += o.Output
	u.Reasoning += o.Reasoning
	u.Calls += o.Calls
	u.Turns += o.Turns
	if o.WeeklyUsed != nil {
		u.WeeklyUsed = o.WeeklyUsed
	}
}

func (u Usage) empty() bool { return u.Input == 0 && u.Output == 0 && u.Calls == 0 }

func count(m map[string]any, key string) int64 {
	n, _ := m[key].(float64)
	return int64(n)
}

// agyUsage adds one finished model call of Antigravity, counted once per step.
func (t *turnState) agyUsage(step string, u map[string]any) {
	if u == nil || t.counted[step] {
		return
	}
	if t.counted == nil {
		t.counted = map[string]bool{}
	}
	t.counted[step] = true
	t.Usage.add(Usage{Input: count(u, "input_tokens"), Cached: count(u, "cache_read_tokens"), Output: count(u, "output_tokens"), Reasoning: count(u, "thinking_tokens"), Calls: 1})
}

// museUsage takes Muse's running total for the turn, which it reports again
// as the turn goes on, along with the account's allowance.
func (t *turnState) museUsage(p map[string]any) {
	var u Usage
	for _, m := range anySlice(p["models"]) {
		model := obj(m)
		u.add(Usage{Input: count(model, "input_tokens"), Cached: count(model, "cached_tokens"), Output: count(model, "output_tokens"), Reasoning: count(model, "reasoning_tokens"), Calls: int(count(model, "calls"))})
	}
	if weekly, ok := obj(obj(p["allowance"])["weekly"])["used_percent"].(float64); ok {
		u.WeeklyUsed = &weekly
	}
	t.Usage = u
}

// usageDay is one agent's spending, today and in total.
type usageDay struct {
	Member string `json:"member"`
	Day    string `json:"day"`
	Today  Usage  `json:"today"`
	Total  Usage  `json:"total"`
}

// recordUsage notes what a member's turn cost: in the audit log, the room's
// run, and the member's running totals.
func (h *APIHandler) recordUsage(ctx context.Context, room, member string, u *Usage) {
	if u == nil || u.empty() {
		return
	}
	u.Turns = 1
	machine := "this computer"
	if peer, _, remote := strings.Cut(member, "::"); remote {
		var p Peer
		if h.peerDoc(ctx, peer, &p) == nil {
			machine = p.Name
		}
	}
	h.audit(member, "TURN_USAGE", room, map[string]any{"machine": machine, "input": u.Input, "cached": u.Cached, "output": u.Output, "reasoning": u.Reasoning, "calls": u.Calls})
	h.sched.Lock()
	rr := h.roomFor(room)
	rr.run.Usage.add(*u)
	h.sched.Unlock()
	h.sessionDocMu.Lock()
	defer h.sessionDocMu.Unlock()
	var d usageDay
	h.store.Document(ctx, "usage", member, &d)
	if today := time.Now().Format("2006-01-02"); d.Day != today {
		d.Day, d.Today = today, Usage{}
	}
	d.Member = member
	d.Today.add(*u)
	d.Total.add(*u)
	h.store.PutDocument(ctx, "usage", member, d)
}

// usage serves GET /api/v1/usage: what each agent on every machine spent
// today and in total.
func (h *APIHandler) usage(w http.ResponseWriter, r *http.Request) {
	docs, err := h.store.Documents(r.Context(), "usage")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	type row struct {
		Name string `json:"name"`
		usageDay
	}
	out := []row{}
	today := time.Now().Format("2006-01-02")
	for _, b := range docs {
		var d usageDay
		if json.Unmarshal(b, &d) != nil || d.Member == "" {
			continue
		}
		if d.Day != today {
			d.Today = Usage{}
		}
		out = append(out, row{Name: h.memberName(r.Context(), d.Member), usageDay: d})
	}
	writeJSON(w, 200, out)
}
