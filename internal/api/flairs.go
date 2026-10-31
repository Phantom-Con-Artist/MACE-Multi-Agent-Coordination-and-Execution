package api

import (
	"context"
	"regexp"
	"slices"
	"strings"
)

// Flairs are short tags at the start of a message, such as [question] or
// [done]. MACE strips the tag and keeps it on the message so people and agents
// can scan or filter a conversation by intent. A group may add a few custom
// flairs of its own; every member sees the group's list each turn.

var standardFlairs = []string{"question", "idea", "plan", "decision", "blocker", "done", "fyi"}

// reservedTags are protocol markers, never flairs.
var reservedTags = []string{"pass", "goal_complete", "share", "dm", "group", "role"}

const maxCustomFlairs = 8

var flairPattern = regexp.MustCompile(`^\s*\[([A-Za-z][A-Za-z0-9 _-]{0,19})\]\s*`)

// splitFlair separates a leading [flair] from the text. A tag that is not
// standard is accepted only when allowed says so.
func splitFlair(text string, allowed func(string) bool) (flair, rest string) {
	m := flairPattern.FindStringSubmatch(text)
	if m == nil {
		return "", text
	}
	name := strings.ToLower(strings.Join(strings.Fields(m[1]), " "))
	key := strings.ReplaceAll(name, " ", "_")
	if slices.Contains(reservedTags, key) || strings.HasPrefix(key, "share") || strings.HasPrefix(key, "dm") {
		return "", text
	}
	if !slices.Contains(standardFlairs, name) && (allowed == nil || !allowed(name)) {
		return "", text
	}
	return name, strings.TrimSpace(text[len(m[0]):])
}

// groupFlairs lists the flairs a group can use: the standard ones and its own.
func groupFlairs(g *Room) []string {
	out := slices.Clone(standardFlairs)
	if g != nil {
		out = append(out, g.Flairs...)
	}
	return out
}

// takeFlair strips a reply's leading flair. In a group, an agent may introduce
// a custom flair; it joins the group's list while there is room.
func (h *APIHandler) takeFlair(ctx context.Context, room string, group *Room, text string) (string, string) {
	if group == nil {
		return splitFlair(text, nil)
	}
	return splitFlair(text, func(name string) bool {
		h.roomMu.Lock()
		defer h.roomMu.Unlock()
		var g Room
		if h.store.Document(ctx, "room", room, &g) != nil {
			return false
		}
		if slices.Contains(g.Flairs, name) {
			return true
		}
		if len(g.Flairs) >= maxCustomFlairs {
			return false
		}
		g.Flairs = append(g.Flairs, name)
		h.store.PutDocument(ctx, "room", room, g)
		h.events.publish("room", g)
		return true
	})
}

var rolePattern = regexp.MustCompile(`^\s*\[role:\s*([^\]\n]{1,160})\]\s*`)

// takeTags strips the leading [role: ...] claim and flair from a reply, in
// either order. A role claim in a group is recorded for every member to see.
func (h *APIHandler) takeTags(ctx context.Context, room string, group *Room, member, text string) (flair, role, rest string) {
	rest = text
	for range 2 {
		if m := rolePattern.FindStringSubmatch(rest); m != nil && group != nil && role == "" {
			role = strings.Join(strings.Fields(m[1]), " ")
			rest = strings.TrimSpace(rest[len(m[0]):])
			continue
		}
		if flair == "" {
			if f, r := h.takeFlair(ctx, room, group, rest); f != "" {
				flair, rest = f, r
				continue
			}
		}
		break
	}
	if role != "" {
		h.roomMu.Lock()
		var g Room
		if h.store.Document(ctx, "room", room, &g) == nil {
			if g.Claimed == nil {
				g.Claimed = map[string]string{}
			}
			g.Claimed[member] = role
			h.store.PutDocument(ctx, "room", room, g)
			h.events.publish("room", g)
		}
		h.roomMu.Unlock()
	}
	return flair, role, rest
}
