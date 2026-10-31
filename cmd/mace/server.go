package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"mace/internal/qrcode"
)

// mace-server runs this machine as the hub the owner's phone opens. It starts
// MACE, publishes it on the tailnet over HTTPS with tailscale serve, and
// prints a one-time sign-in QR code. The same commands work as mace server.

const serverHelp = `Usage: mace-server <command>

  start [--port 8080]   Start MACE, serve it on your tailnet over HTTPS, and print a sign-in QR code
  qr                    Print a fresh one-time sign-in QR code
  status                Show the server and its tailnet address
  stop                  Stop serving on the tailnet and stop MACE
`

type tailscaleSelf struct {
	BackendState string `json:"BackendState"`
	Self         struct {
		DNSName string `json:"DNSName"`
	} `json:"Self"`
	CertDomains []string `json:"CertDomains"`
}

// tailnetName is this machine's HTTPS name on the tailnet.
func tailnetName() (string, error) {
	out, err := exec.Command("tailscale", "status", "--json").Output()
	if err != nil {
		return "", errors.New("Tailscale is not installed or not running here. Install it, run sudo tailscale up, then mace-server start again")
	}
	return parseTailnetName(out)
}

func parseTailnetName(out []byte) (string, error) {
	var st tailscaleSelf
	if err := json.Unmarshal(out, &st); err != nil {
		return "", err
	}
	if st.BackendState != "Running" {
		return "", fmt.Errorf("Tailscale is %s. Run sudo tailscale up, then mace-server start again", strings.ToLower(st.BackendState))
	}
	name := strings.TrimSuffix(st.Self.DNSName, ".")
	if len(st.CertDomains) > 0 {
		name = st.CertDomains[0]
	}
	if name == "" {
		return "", errors.New("this machine has no tailnet name. Turn on MagicDNS and HTTPS certificates in the Tailscale admin console (DNS page)")
	}
	return name, nil
}

func serverCommand(args []string) error {
	command := "help"
	if len(args) > 0 {
		command, args = args[0], args[1:]
	}
	if command == "help" || command == "-h" || command == "--help" {
		fmt.Print(serverHelp)
		return nil
	}
	c, err := loadSettings(args)
	if err != nil {
		return err
	}
	switch command {
	case "start":
		return c.serverStart()
	case "qr":
		return c.printSignIn()
	case "status":
		if err := c.status(); err != nil {
			return err
		}
		out, _ := exec.Command("tailscale", "serve", "status").CombinedOutput()
		fmt.Print(string(out))
		return nil
	case "stop":
		if out, err := exec.Command("tailscale", "serve", "--https=443", "off").CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "Could not stop serving on the tailnet: %s\n", strings.TrimSpace(string(out)))
		}
		return c.stop(false)
	}
	return fmt.Errorf("unknown command %q; run mace-server help", command)
}

func (c settings) serverStart() error {
	name, err := tailnetName()
	if err != nil {
		return err
	}
	public := "https://" + name
	// Other machines and sign-in links use the HTTPS address.
	os.Setenv("MACE_PUBLIC_URL", public)
	if c.owns(c.pid()) {
		fmt.Println("MACE is already running here; serving it on the tailnet.")
	} else if err := c.start(); err != nil {
		return err
	}
	target := "http://127.0.0.1:" + strconv.Itoa(c.Port)
	out, err := exec.Command("tailscale", "serve", "--bg", "--https=443", target).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(strings.ToLower(msg), "access denied") || strings.Contains(msg, "operator") {
			return fmt.Errorf("Tailscale needs permission to serve for you. Run sudo tailscale set --operator=$USER once, then mace-server start again.\n%s", msg)
		}
		return fmt.Errorf("tailscale serve did not start: %s", msg)
	}
	fmt.Printf("\nServing on your tailnet: %s\n", public)
	return c.printSignIn()
}

// printSignIn asks the running server for a one-time sign-in link and prints
// it with its QR code.
func (c settings) printSignIn() error {
	var link struct {
		Link string `json:"link"`
	}
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		if err = c.send("GET", "/signin/qr", 10*time.Second, nil, &link); err == nil {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if err != nil {
		return err
	}
	code, err := qrcode.Terminal(link.Link)
	if err != nil {
		return err
	}
	fmt.Printf("\nScan to sign in your phone (works once, for 10 minutes):\n\n%s\n%s\n\nThen use Add to Home Screen. A fresh code: mace-server qr\n", code, link.Link)
	return nil
}
