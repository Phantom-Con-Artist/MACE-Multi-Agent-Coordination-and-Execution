package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

type nodeView struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	URL       string     `json:"url"`
	Self      bool       `json:"self"`
	Available bool       `json:"available"`
	LastSeen  *time.Time `json:"last_seen"`
	Problem   string     `json:"problem"`
	Checking  bool       `json:"checking"`
}

// join prints an invite with --invite, or joins this machine to another one.
func (c settings) join(args []string) error {
	rest := []string{}
	invite := false
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--invite":
			invite = true
		case a == "--port" || a == "--host":
			i++
		case strings.HasPrefix(a, "-"):
		default:
			rest = append(rest, a)
		}
	}
	if invite {
		var inv struct {
			Code    string    `json:"code"`
			URL     string    `json:"url"`
			Expires time.Time `json:"expires"`
		}
		if err := c.send("POST", "/nodes/invite", 10*time.Second, map[string]any{}, &inv); err != nil {
			return err
		}
		fmt.Printf("Join code: %s (works once, until %s)\n\nOn the machine you are adding, run:\n\n  mace join %s %s\n\n", inv.Code, inv.Expires.Local().Format("15:04"), inv.URL, inv.Code)
		return nil
	}
	if len(rest) != 2 {
		return fmt.Errorf("usage: mace join <address> <code>, with both printed by mace join --invite on a machine already set up")
	}
	url := rest[0]
	if !strings.HasPrefix(url, "http") {
		url = "http://" + url
	}
	var out struct {
		Node  nodeView `json:"node"`
		Known int      `json:"known"`
	}
	if err := c.send("POST", "/nodes/join", 30*time.Second, map[string]string{"url": url, "code": rest[1]}, &out); err != nil {
		return err
	}
	fmt.Printf("Joined %s. This machine now knows %d other %s; mace machines lists them.\n", out.Node.Name, out.Known, plural(out.Known, "machine", "machines"))
	return nil
}

func (c settings) nodes() error {
	var list []nodeView
	if err := c.request("/nodes", &list); err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "MACHINE\tSTATE\tADDRESS\tLAST SEEN")
	for _, n := range list {
		state, seen := "unavailable", "never"
		if n.Available {
			state = "available"
		} else if n.Checking {
			state = "checking"
		}
		if n.Self {
			state = "this machine"
		}
		if n.Problem != "" && !n.Self {
			state += " (" + strings.ToLower(n.Problem) + ")"
		}
		if n.LastSeen != nil {
			seen = n.LastSeen.Local().Format("15:04:05")
		}
		if n.Self {
			seen = "now"
		}
		url := n.URL
		if url == "" {
			url = "not reachable by other machines (start Tailscale)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", n.Name, state, url, seen)
	}
	return w.Flush()
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
