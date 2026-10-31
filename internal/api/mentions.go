package api

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

var handleUnsafe = regexp.MustCompile(`[^a-z0-9._-]+`)

// slug turns a name into a mention handle: lowercase, no spaces.
func slug(name string) string {
	s := strings.Trim(handleUnsafe.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-"), "-._")
	if s == "" {
		return "agent"
	}
	return limited(s, 32)
}

// nextHandle picks the first free handle for a provider on this host: agy,
// agy-2, agy-3, and so on.
func (h *APIHandler) nextHandle(ctx context.Context, base, except string) string {
	taken := map[string]bool{"all": true, "you": true, "owner": true, "mace": true}
	raw, _ := h.store.Documents(ctx, "session")
	for _, b := range raw {
		var s Session
		if json.Unmarshal(b, &s) == nil && s.ID != except && s.Status != "removed" && s.Handle != "" {
			taken[s.Handle] = true
		}
	}
	base = slug(base)
	if !taken[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := base + "-" + strconv.Itoa(i)
		if !taken[candidate] {
			return candidate
		}
	}
}

// ensureHandles gives sessions created before handles existed a handle.
func (h *APIHandler) ensureHandles(ctx context.Context) {
	raw, _ := h.store.Documents(ctx, "session")
	for _, b := range raw {
		var s Session
		if json.Unmarshal(b, &s) != nil || s.Handle != "" {
			continue
		}
		h.updateSession(ctx, s.ID, func(s *Session) { s.Handle = h.nextHandle(ctx, s.Provider, s.ID) })
	}
}

// memberHandle returns the handle a member is known by, before any in-room
// disambiguation.
func (h *APIHandler) memberHandle(ctx context.Context, id string) string {
	var s Session
	if h.store.Document(ctx, "session", id, &s) == nil && s.Handle != "" {
		return s.Handle
	}
	if h.store.Document(ctx, "remote-session", id, &s) == nil {
		if s.Handle != "" {
			return s.Handle
		}
		_, name, _ := strings.Cut(s.Name, " / ")
		return slug(name)
	}
	return slug(h.memberName(ctx, id))
}

// roomHandles maps each member to a handle that is unique in the room. Remote
// members whose handle collides get their host's name appended, as agy@pc.
func (h *APIHandler) roomHandles(ctx context.Context, members []string) map[string]string {
	out := map[string]string{}
	count := map[string]int{}
	for _, m := range members {
		out[m] = h.memberHandle(ctx, m)
		count[out[m]]++
	}
	for _, m := range members {
		if count[out[m]] < 2 {
			continue
		}
		if peer, _, remote := strings.Cut(m, "::"); remote {
			var p Peer
			suffix := peer
			if h.store.Document(ctx, "peer", peer, &p) == nil {
				suffix = slug(p.Name)
			}
			out[m] = out[m] + "@" + suffix
		} else {
			out[m] = out[m] + "@" + "here"
		}
	}
	return out
}

var mentionPattern = regexp.MustCompile(`(?:^|[^A-Za-z0-9_])@([A-Za-z0-9][A-Za-z0-9._@-]*[A-Za-z0-9]|[A-Za-z0-9])`)

// parseMentions returns the members addressed in text and whether @all was
// used. Matching is case-insensitive and ignores trailing punctuation.
var codeSpans = regexp.MustCompile("(?s)```.*?```|`[^`\n]*`")

var leadMention = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_@])@lead\b`)

// roomMentions parses mentions in a conversation; in a team, @lead names the lead.
func (h *APIHandler) roomMentions(ctx context.Context, room, text string, members []string) ([]string, bool) {
	ids, all := parseMentions(text, h.roomHandles(ctx, members))
	if p := h.policyFor(ctx, room, members); p.Mode == "team" && p.Lead != "" && !containsID(ids, p.Lead) &&
		leadMention.MatchString(codeSpans.ReplaceAllString(text, " ")) {
		ids = append(ids, p.Lead)
	}
	return ids, all
}

func parseMentions(text string, handles map[string]string) (members []string, all bool) {
	byHandle := map[string]string{}
	for member, handle := range handles {
		byHandle[strings.ToLower(handle)] = member
	}
	seen := map[string]bool{}
	// A handle quoted in code, such as `@agy` in a file listing, is not a mention.
	text = codeSpans.ReplaceAllString(text, " ")
	for _, match := range mentionPattern.FindAllStringSubmatch(text, -1) {
		name := strings.ToLower(match[1])
		if name == "all" || name == "everyone" {
			all = true
			continue
		}
		member, ok := byHandle[name]
		if !ok {
			// "@agy@pc" may also be written without the host when unique.
			if base, _, cut := strings.Cut(name, "@"); cut {
				member, ok = byHandle[base]
			}
		}
		if ok && !seen[member] {
			seen[member] = true
			members = append(members, member)
		}
	}
	return members, all
}

// updateSession applies a change to a stored session under a lock, so
// concurrent turns and controls never overwrite each other's fields.
func (h *APIHandler) updateSession(ctx context.Context, id string, change func(*Session)) (Session, error) {
	h.sessionDocMu.Lock()
	defer h.sessionDocMu.Unlock()
	var s Session
	if err := h.store.Document(ctx, "session", id, &s); err != nil {
		return s, err
	}
	change(&s)
	if err := h.store.PutDocument(context.Background(), "session", id, s); err != nil {
		return s, err
	}
	h.events.publish("session", s)
	return s, nil
}
