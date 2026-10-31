// Package qrcode draws sign-in links as QR codes for a terminal or a page.
package qrcode

import (
	"fmt"
	"strings"

	"rsc.io/qr"
)

// quiet is the blank border, in modules, that scanners need around a code.
const quiet = 2

// Terminal draws a code with half-block characters, two rows per line, dark
// modules on a light background so it scans in dark and light terminals.
func Terminal(text string) (string, error) {
	c, err := qr.Encode(text, qr.L)
	if err != nil {
		return "", err
	}
	dark := func(x, y int) bool { return c.Black(x-quiet, y-quiet) }
	size := c.Size + 2*quiet
	var b strings.Builder
	for y := 0; y < size; y += 2 {
		b.WriteString("\x1b[30;47m")
		for x := 0; x < size; x++ {
			top, bottom := dark(x, y), y+1 < size && dark(x, y+1)
			switch {
			case top && bottom:
				b.WriteString("█")
			case top:
				b.WriteString("▀")
			case bottom:
				b.WriteString("▄")
			default:
				b.WriteString(" ")
			}
		}
		b.WriteString("\x1b[0m\n")
	}
	return b.String(), nil
}

// SVG draws a code as one path, sized by the page.
func SVG(text string) (string, error) {
	c, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", err
	}
	size := c.Size + 2*quiet
	var path strings.Builder
	for y := 0; y < c.Size; y++ {
		for x := 0; x < c.Size; x++ {
			if c.Black(x, y) {
				fmt.Fprintf(&path, "M%d %dh1v1h-1z", x+quiet, y+quiet)
			}
		}
	}
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges"><rect width="100%%" height="100%%" fill="#fff"/><path fill="#000" d="%s"/></svg>`, size, size, path.String()), nil
}
