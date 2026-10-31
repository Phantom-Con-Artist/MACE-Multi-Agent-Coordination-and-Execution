package main

import (
	"context"
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
  setup [--remove]      Run the hub as a background service that starts at boot
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
	// Without HTTPS certificates tailscale serve waits for them to be turned
	// on instead of failing, so check first.
	if len(st.CertDomains) == 0 || st.CertDomains[0] == "" {
		return "", errors.New("HTTPS is off for your tailnet. Turn on MagicDNS and HTTPS Certificates at https://login.tailscale.com/admin/dns, then run this again")
	}
	return st.CertDomains[0], nil
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
	case "setup":
		return c.serverSetup(args)
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
	if err := c.serve(); err != nil {
		return err
	}
	fmt.Printf("\nServing on your tailnet: %s\nDevices logged in to your Tailscale account sign in by opening it, and your other machines running MACE join by themselves.\n", public)
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
	fmt.Printf("\nOr scan this to sign in (works once, for 10 minutes):\n\n%s\n%s\n\nThen use Add to Home Screen. A fresh code: mace-server qr\n", code, link.Link)
	return nil
}

// serverSetup installs the hub as a service. tailscale serve keeps its
// configuration across reboots, so the HTTPS address comes back with it.
func (c settings) serverSetup(args []string) error {
	for _, a := range args {
		if a == "--remove" {
			exec.Command("tailscale", "serve", "--https=443", "off").Run()
			return c.setup(args)
		}
	}
	name, err := tailnetName()
	if err != nil {
		return err
	}
	os.Setenv("MACE_PUBLIC_URL", "https://"+name)
	if err := c.setup(args); err != nil {
		return err
	}
	if err := c.serve(); err != nil {
		return err
	}
	fmt.Printf("\nThe hub is at https://%s. Open it on any device logged in to your Tailscale account; it signs in by itself.\n", name)
	return nil
}

func (c settings) serve() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tailscale", "serve", "--bg", "--https=443", "http://127.0.0.1:"+strconv.Itoa(c.Port)).CombinedOutput()
	if ctx.Err() != nil {
		return errors.New("tailscale serve did not finish in 30 seconds; check that HTTPS is on for your tailnet, then run this again")
	}
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if strings.Contains(strings.ToLower(msg), "access denied") || strings.Contains(msg, "operator") {
		return fmt.Errorf("Tailscale needs permission to serve for you. Run sudo tailscale set --operator=$USER once, then try again.\n%s", msg)
	}
	return fmt.Errorf("tailscale serve did not start: %s", msg)
}
