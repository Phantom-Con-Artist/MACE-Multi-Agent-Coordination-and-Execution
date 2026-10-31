package qrcode

import (
	"strings"
	"testing"
)

func TestCodesDrawForTerminalsAndPages(t *testing.T) {
	link := "https://fedora.example.ts.net/#connect=abc123"
	term, err := Terminal(link)
	if err != nil || strings.Count(term, "\n") < 10 || !strings.Contains(term, "█") {
		t.Fatalf("terminal code: %v\n%s", err, term)
	}
	svg, err := SVG(link)
	if err != nil || !strings.HasPrefix(svg, "<svg") || !strings.Contains(svg, "M2 2h1v1h-1z") {
		t.Fatalf("svg code: %v %s", err, svg[:min(len(svg), 200)])
	}
}
