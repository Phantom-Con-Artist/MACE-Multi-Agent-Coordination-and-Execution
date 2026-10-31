package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const ctxHelp = `Usage: mace ctx <view> [words]

Fetch conversation context during an agent turn, instead of receiving all of it every turn.

  team              Who is in this group, their roles, and how the team works
  plan              The owner's requests, plans, decisions, role claims, and done reports
  recent [n]        The newest n messages (default 20)
  thread @handle    What one coworker said or was told
  search <words>    Messages containing all the words

  --chat <id>       Read another conversation you belong to
`

// contextFetch runs inside an agent's turn. Mace gives the agent's process
// its address, the agent's id, a read-only token, and the conversation.
func contextFetch(args []string) error {
	chat := os.Getenv("MACE_CTX_CHAT")
	rest := []string{}
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--chat" && i+1 < len(args):
			chat = args[i+1]
			i++
		case a == "-h" || a == "--help" || a == "help":
			fmt.Print(ctxHelp)
			return nil
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) == 0 {
		fmt.Print(ctxHelp)
		return nil
	}
	base, agent, token := os.Getenv("MACE_CTX_URL"), os.Getenv("MACE_CTX_AGENT"), os.Getenv("MACE_CTX_TOKEN")
	if base == "" || agent == "" || token == "" || chat == "" {
		return errors.New("mace ctx works inside an agent turn that MACE started")
	}
	u := strings.TrimRight(base, "/") + "/api/v1/ctx/" + url.PathEscape(chat) + "/" + url.PathEscape(rest[0])
	if q := strings.Join(rest[1:], " "); q != "" {
		u += "?q=" + url.QueryEscape(q)
	}
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Mace-Agent", agent)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("could not reach MACE: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		return errors.New(strings.TrimSpace(string(body)))
	}
	os.Stdout.Write(body)
	return nil
}
