package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Agent health: an agent close to its quota, with a nearly full context, or
// on a machine about to run out of battery is told so each turn, and told to
// hand its job on before it fails halfway; the lead sees the same for every
// job owner. Quota and context come from what the CLIs report per turn
// (usage.go) and from their errors; battery is read on the machine itself.

// AgentHealth is what MACE last saw of an agent's limits.
type AgentHealth struct {
	Member     string    `json:"member"`
	Provider   string    `json:"provider"`
	WeeklyUsed *float64  `json:"weekly_used,omitempty"`
	QuotaNote  string    `json:"quota_note,omitempty"`
	QuotaAt    time.Time `json:"quota_at,omitempty"`
	// Context is the input of the agent's last turn, which for a resumed
	// native session is about how full its context is.
	Context int64     `json:"context,omitempty"`
	Updated time.Time `json:"updated"`
}

// Battery is a machine's battery, when it has one.
type Battery struct {
	Percent  int  `json:"percent"`
	Charging bool `json:"charging"`
}

// contextWindows are the context sizes of each CLI's default model, in
// tokens, for a percentage; a CLI not listed is shown in tokens only.
var contextWindows = map[string]int64{"agy": 1_000_000, "claude": 200_000, "codex": 272_000}

var quotaWords = regexp.MustCompile(`(?i)(quota|rate[ -]?limit|usage limit|too many requests|\b429\b|credits? (are )?exhausted|out of credits|resource[_ ]exhausted|limit reached)`)

// noteHealth records what a finished turn showed of an agent's limits.
func (h *APIHandler) noteHealth(ctx context.Context, member string, u *Usage, turnErr error) {
	var hl AgentHealth
	h.store.Document(ctx, "agent-health", member, &hl)
	hl.Member = member
	if hl.Provider == "" {
		var s Session
		if h.store.Document(ctx, "session", member, &s) == nil || h.store.Document(ctx, "remote-session", member, &s) == nil {
			hl.Provider = s.Provider
		}
	}
	if u != nil {
		if u.WeeklyUsed != nil {
			hl.WeeklyUsed = u.WeeklyUsed
		}
		if u.Input > 0 {
			hl.Context = u.Input
		}
	}
	if turnErr != nil && quotaWords.MatchString(turnErr.Error()) {
		hl.QuotaNote, hl.QuotaAt = limited(strings.TrimSpace(turnErr.Error()), 160), time.Now().UTC()
	}
	hl.Updated = time.Now().UTC()
	h.store.PutDocument(ctx, "agent-health", member, hl)
}

// healthWarnings lists what is wrong with an agent's limits, if anything.
func (h *APIHandler) healthWarnings(ctx context.Context, member string) []string {
	var hl AgentHealth
	out := []string{}
	if h.store.Document(ctx, "agent-health", h.localize(member), &hl) == nil {
		if hl.WeeklyUsed != nil && *hl.WeeklyUsed >= 80 {
			out = append(out, fmt.Sprintf("weekly quota %.0f%% used", *hl.WeeklyUsed))
		}
		if hl.QuotaNote != "" && time.Since(hl.QuotaAt) < 6*time.Hour {
			out = append(out, "its CLI reported a limit "+ago(time.Since(hl.QuotaAt))+" ago")
		}
		if window := contextWindows[hl.Provider]; window > 0 && hl.Context*100/window >= 70 {
			out = append(out, fmt.Sprintf("context %d%% full", hl.Context*100/window))
		}
	}
	if f, ok := h.factsOf(ctx, member); ok && f.Battery != nil && !f.Battery.Charging && f.Battery.Percent <= 25 {
		out = append(out, fmt.Sprintf("its machine is on battery at %d%%", f.Battery.Percent))
	}
	return out
}

// healthLine is the warning an agent reads about itself, empty when all is well.
func (h *APIHandler) healthLine(ctx context.Context, s Session) string {
	w := h.healthWarnings(ctx, s.ID)
	if len(w) == 0 {
		return ""
	}
	return "YOUR LIMITS: " + strings.Join(w, "; ") + ". If you may not finish what you hold, hand it on now: [pass <id> @handle] to a coworker whose machine fits, or [drop <id>] with why, and tell your lead.\n"
}

// readBattery reads this machine's battery from sysfs; nil without one.
func readBattery() *Battery {
	dirs, _ := filepath.Glob("/sys/class/power_supply/*")
	var b *Battery
	charging := false
	for _, d := range dirs {
		kind, _ := os.ReadFile(filepath.Join(d, "type"))
		switch strings.TrimSpace(string(kind)) {
		case "Battery":
			if c, err := os.ReadFile(filepath.Join(d, "capacity")); err == nil {
				p, _ := strconv.Atoi(strings.TrimSpace(string(c)))
				b = &Battery{Percent: p}
				if st, err := os.ReadFile(filepath.Join(d, "status")); err == nil && strings.TrimSpace(string(st)) != "Discharging" {
					charging = true
				}
			}
		case "Mains", "USB":
			if on, err := os.ReadFile(filepath.Join(d, "online")); err == nil && strings.TrimSpace(string(on)) == "1" {
				charging = true
			}
		}
	}
	if b != nil {
		b.Charging = charging
	}
	return b
}
