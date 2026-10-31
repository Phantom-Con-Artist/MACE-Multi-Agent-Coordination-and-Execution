package api

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"
)

// Turn caps stop runaway chains but not an agent saying the same thing
// again in slightly different words. A reply to coworkers that is nearly
// a copy of one of the agent's own recent messages is treated as [PASS].
// Similarity is the Jaccard index of word shingles (pairs of words), the
// standard near-duplicate test from Broder's work on web pages.

const (
	repeatSimilarity = 0.7
	repeatLookback   = 3
)

func shingles(text string) map[string]bool {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	set := map[string]bool{}
	if len(words) == 1 {
		set[words[0]] = true
	}
	for i := 0; i+1 < len(words); i++ {
		set[words[i]+" "+words[i+1]] = true
	}
	return set
}

func similarity(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	shared := 0
	for k := range a {
		if b[k] {
			shared++
		}
	}
	return float64(shared) / float64(len(a)+len(b)-shared)
}

// repeats reports whether text nearly copies one of member's last messages
// in room.
func (h *APIHandler) repeats(ctx context.Context, room, member, text string) bool {
	rows, _, err := h.store.Messages(ctx, room, 0, 0, 40)
	if err != nil {
		return false
	}
	mine := shingles(text)
	seen := 0
	for _, row := range rows {
		var m Message
		if json.Unmarshal(row.Data, &m) != nil || m.Sender != member || m.Text == "" {
			continue
		}
		if similarity(mine, shingles(m.Text)) >= repeatSimilarity {
			return true
		}
		if seen++; seen >= repeatLookback {
			break
		}
	}
	return false
}
