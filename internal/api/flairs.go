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
var reservedTags = []string{"pass", "goal_complete", "share", "dm"}

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
