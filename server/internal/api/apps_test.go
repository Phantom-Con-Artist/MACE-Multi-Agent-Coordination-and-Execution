package api

import "testing"

func TestMainCategoryPicksTheFirstKnownOne(t *testing.T) {
	for in, want := range map[string]string{"Graphics;3DGraphics;": "Graphics", "Qt;KDE;Network;WebBrowser;": "Internet", "X-Custom;": "Other", "": "Other"} {
		if got := mainCategory(in); got != want {
			t.Errorf("mainCategory(%q) = %q, want %q", in, got, want)
		}
	}
}
