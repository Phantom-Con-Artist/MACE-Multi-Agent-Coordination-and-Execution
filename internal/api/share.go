package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Agents share files instead of printing them. [share: path] in a reply
// attaches the file; a long pasted code block becomes a file automatically.
// Tagged agents get the path in their next turn; everyone else only learns
// that something was shared, so nobody reads files that are not for them.

var (
	sharePattern = regexp.MustCompile(`\[share:\s*([^\]\n]+)\]`)
	fencePattern = regexp.MustCompile("(?s)```([A-Za-z0-9_+-]*)\n(.*?)```")
)

// longPaste is the size above which a fenced code block becomes a file.
const longPaste = 2500

const maxSharesPerReply = 5

var fenceExtensions = map[string]string{"go": ".go", "js": ".js", "javascript": ".js", "ts": ".ts", "typescript": ".ts", "tsx": ".tsx", "py": ".py", "python": ".py", "json": ".json", "md": ".md", "markdown": ".md", "sh": ".sh", "bash": ".sh", "yaml": ".yaml", "yml": ".yaml", "rust": ".rs", "rs": ".rs", "html": ".html", "css": ".css", "sql": ".sql"}

// rootRelative turns an absolute path into the form messages store, relative
// to the shared file root, or reports that it lies outside it.
func (h *APIHandler) rootRelative(abs string) (string, bool) {
	rel, err := filepath.Rel(h.fileRoot, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func (h *APIHandler) sharesDir(room string) string {
	base := os.TempDir()
	if p := h.store.Path(); p != ":memory:" {
		base = filepath.Dir(p)
	}
	return filepath.Join(base, "shares", room)
}

// attachShares removes share tags and long pastes from a reply and returns
// the files to attach, relative to the file root. Remote agents' paths name
// files on their own host, so their tags are left as text.
func (h *APIHandler) attachShares(ctx context.Context, m *Message, s Session) []string {
	files := []string{}
	if strings.Contains(s.ID, "::") {
		return files
	}
	m.Text = sharePattern.ReplaceAllStringFunc(m.Text, func(tag string) string {
		raw := strings.TrimSpace(sharePattern.FindStringSubmatch(tag)[1])
		path := raw
		if strings.HasPrefix(path, "~") {
			home, _ := os.UserHomeDir()
			path = filepath.Join(home, strings.TrimPrefix(path, "~"))
		} else if !filepath.IsAbs(path) {
			path = filepath.Join(s.Directory, path)
		}
		path = filepath.Clean(path)
		info, err := os.Stat(path)
		rel, inside := h.rootRelative(path)
		if err != nil || !info.Mode().IsRegular() || !inside || len(files) >= maxSharesPerReply {
			return "(could not share " + raw + ")"
		}
		if !slices.Contains(files, rel) {
			files = append(files, rel)
		}
		return ""
	})
	m.Text = fencePattern.ReplaceAllStringFunc(m.Text, func(block string) string {
		parts := fencePattern.FindStringSubmatch(block)
		if len(parts[2]) < longPaste || len(files) >= maxSharesPerReply {
			return block
		}
		dir := h.sharesDir(m.Room)
		if os.MkdirAll(dir, 0700) != nil {
			return block
		}
		ext := fenceExtensions[strings.ToLower(parts[1])]
		if ext == "" {
			ext = ".txt"
		}
		sum := sha256.Sum256([]byte(parts[2]))
		name := time.Now().UTC().Format("20060102-150405") + "-" + hex.EncodeToString(sum[:4]) + ext
		path := filepath.Join(dir, name)
		rel, inside := h.rootRelative(path)
		if !inside || os.WriteFile(path, []byte(parts[2]), 0600) != nil {
			return block
		}
		files = append(files, rel)
		return fmt.Sprintf("(shared as %s, %d lines)", name, strings.Count(parts[2], "\n"))
	})
	m.Text = strings.TrimSpace(regexp.MustCompile(`[ \t]+\n`).ReplaceAllString(m.Text, "\n"))
	return files
}

// fileLine is how a shared file appears in a reader's transcript.
func (h *APIHandler) fileLine(ctx context.Context, m Message, reader string) string {
	name := filepath.Base(m.File)
	if len(m.Mentions) > 0 && reader != m.Sender && !slices.Contains(m.Mentions, reader) {
		handles := h.roomHandles(ctx, m.Mentions)
		tagged := []string{}
		for _, id := range m.Mentions {
			tagged = append(tagged, "@"+handles[id])
		}
		return fmt.Sprintf("%s shared %s with %s. It is not for you; ask if you need it.\n", h.memberName(ctx, m.Sender), name, strings.Join(tagged, ", "))
	}
	return fmt.Sprintf("Shared file %s on the originating host: %s\n", name, filepath.Join(h.fileRoot, m.File))
}

// Side chats let two agents in a group talk privately. The owner can read
// and join them; [group] in a side chat reply posts it back to the group.

var (
	dmPattern    = regexp.MustCompile(`^\s*\[dm\s+@([A-Za-z0-9._-]+)\]\s*`)
	groupPattern = regexp.MustCompile(`^\s*\[group\]\s*`)
)

// sideRoom returns the side chat between two members of a group, creating it
// on first use.
func (h *APIHandler) sideRoom(ctx context.Context, parent *Room, a, b string) (Room, bool, error) {
	pair := []string{a, b}
	slices.Sort(pair)
	sum := sha256.Sum256([]byte(parent.ID + "|" + pair[0] + "|" + pair[1]))
	id := "side_" + hex.EncodeToString(sum[:12])
	h.roomMu.Lock()
	defer h.roomMu.Unlock()
	var g Room
	if h.store.Document(ctx, "room", id, &g) == nil && !g.Removed {
		return g, false, nil
	}
	g = Room{ID: id, Name: h.memberName(ctx, a) + " and " + h.memberName(ctx, b), Members: []string{a, b}, Parent: parent.ID}
	if err := h.store.PutDocument(ctx, "room", id, g); err != nil {
		return g, false, err
	}
	h.store.PutDocument(ctx, "policy", id, Policy{Mode: "reply", Limit: 12, Tone: h.policyFor(ctx, parent.ID, parent.Members).Tone})
	h.events.publish("room", g)
	return g, true, nil
}

// sideRooms lists the side chats of a group.
func (h *APIHandler) sideRooms(ctx context.Context, parent string) []string {
	out := []string{}
	raw, _ := h.store.Documents(ctx, "room")
	for _, b := range raw {
		var g Room
		if json.Unmarshal(b, &g) == nil && g.Parent == parent && !g.Removed {
			out = append(out, g.ID)
		}
	}
	return out
}

// routeReply sends a reply to a side chat ([dm @handle]) or from a side chat
// back to its group ([group]). It reports whether it handled the reply.
func (h *APIHandler) routeReply(ctx context.Context, group *Room, member string, reply *Message) bool {
	if group == nil {
		return false
	}
	if group.Parent != "" {
		m := groupPattern.FindString(reply.Text)
		if m == "" {
			return false
		}
		var parent Room
		if h.store.Document(ctx, "room", group.Parent, &parent) != nil || parent.Removed {
			return false
		}
		out := *reply
		out.ID, out.Room, out.Text = newID(), parent.ID, strings.TrimSpace(reply.Text[len(m):])
		out.Mentions, out.MentionsAll = parseMentions(out.Text, h.roomHandles(ctx, parent.Members))
		if _, err := h.postMessage(ctx, &out); err == nil {
			h.dispatch(ctx, parent.ID, out)
		}
		return true
	}
	m := dmPattern.FindStringSubmatch(reply.Text)
	if m == nil {
		return false
	}
	target := ""
	for id, handle := range h.roomHandles(ctx, group.Members) {
		if strings.EqualFold(handle, m[1]) && id != member {
			target = id
		}
	}
	if target == "" {
		return false
	}
	side, created, err := h.sideRoom(ctx, group, member, target)
	if err != nil {
		return false
	}
	if created {
		h.commandNote(group.ID, fmt.Sprintf("%s opened a side chat with %s. You can read and join it.", h.memberName(ctx, member), h.memberName(ctx, target)), false)
	}
	out := *reply
	out.ID, out.Room, out.Text = newID(), side.ID, strings.TrimSpace(reply.Text[len(m[0]):])
	out.Mentions, out.MentionsAll, out.Hop = []string{target}, false, 1
	if _, err := h.postMessage(ctx, &out); err == nil {
		h.dispatch(ctx, side.ID, out)
	}
	return true
}
