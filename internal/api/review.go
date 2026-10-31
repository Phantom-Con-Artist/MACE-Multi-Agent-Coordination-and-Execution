package api

import (
	"context"
	"strings"
	"time"
)

// A team lead's [done] does not end the task by itself. Leads judge their
// own team's work generously, so the result goes to the coworker who owns
// its design quality, and only that reviewer's [approve] finishes it, the
// way a critic agent gates termination in other multi-agent systems. The
// reviewer's notes go to the lead, who fixes and reports [done] again.

// maxReviews bounds review rounds; after that the lead's [done] stands.
const maxReviews = 3

var reviewRoles = []string{"design", "review", "qa", "quality", "test"}

// reviewer picks who checks a team's result: the coworker whose role names
// design or review, else the first other coworker who can answer. A lead
// working alone has no reviewer.
func (h *APIHandler) reviewer(ctx context.Context, group *Room, p Policy, members []string) string {
	fallback := ""
	for _, id := range members {
		if id == p.Lead || !h.available(ctx, id) {
			continue
		}
		if fallback == "" {
			fallback = id
		}
		role := strings.ToLower(h.profileRole(ctx, id))
		if group != nil {
			role += " " + strings.ToLower(group.Roles[id]+" "+group.Claimed[id])
		}
		for _, word := range reviewRoles {
			if strings.Contains(role, word) {
				return id
			}
		}
	}
	return fallback
}

// askReview hands the lead's finished result to the reviewer. It reports
// false when the lead's [done] should stand.
func (h *APIHandler) askReview(ctx context.Context, room string, group *Room, p Policy, members []string) bool {
	who := h.reviewer(ctx, group, p, members)
	h.sched.Lock()
	rr := h.roomFor(room)
	ask := who != "" && rr.reviews < maxReviews
	if ask {
		rr.reviews++
		rr.review = who
	} else {
		rr.review = ""
	}
	h.sched.Unlock()
	if !ask {
		return false
	}
	handle := h.roomHandles(ctx, members)[who]
	m := Message{ID: newID(), Room: room, Sender: "mace", Kind: "system", At: time.Now().UTC(), Delivery: map[string]string{}, Mentions: []string{who},
		Text: "@" + handle + " the lead reports the task done. Before the owner hears that, check it as its design owner: open or run the result itself, compare it with the owner's request, and look for anything broken, unfinished, or generic. Reply [approve] if you would ship it as it is; otherwise tell @lead exactly what must change."}
	if _, err := h.postMessage(ctx, &m); err != nil {
		return false
	}
	h.dispatch(ctx, room, m)
	return true
}

// approved reports whether member was the reviewer the task waited for,
// and clears the wait.
func (h *APIHandler) approved(room, member string) bool {
	h.sched.Lock()
	defer h.sched.Unlock()
	rr := h.roomFor(room)
	if rr.review == "" || rr.review != member {
		return false
	}
	rr.review = ""
	return true
}
