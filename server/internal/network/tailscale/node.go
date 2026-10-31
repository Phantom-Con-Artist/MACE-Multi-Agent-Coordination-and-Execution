package tailscale

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"tailscale.com/tsnet"
)

// Config holds the configuration for the Tailscale tsnet node.
type Config struct {
	Host       string
	AuthKey    string
	Hostname   string
	StateDir   string
	ControlURL string
	Ephemeral  bool
	Port       int
}

// Node wraps an embedded user-space Tailscale node or local fallback listener.
type Node struct {
	cfg      Config
	tsServer *tsnet.Server
	listener net.Listener
}

// NewNode initializes a Tailscale tsnet node. If AuthKey is empty, it operates in local fallback mode.
func NewNode(cfg Config) (*Node, error) {
	if cfg.Port < 0 {
		cfg.Port = 8080
	}
	if cfg.Port == 0 && cfg.AuthKey != "" {
		cfg.Port = 8080
	}
	if cfg.Hostname == "" {
		cfg.Hostname = "mace-node"
	}

	node := &Node{cfg: cfg}

	if cfg.AuthKey != "" {
		stateDir := cfg.StateDir
		if stateDir == "" {
			stateDir = "./data/tailscale"
		}
		if err := os.MkdirAll(stateDir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create tailscale state dir: %w", err)
		}

		tsServer := &tsnet.Server{
			Hostname:   cfg.Hostname,
			AuthKey:    cfg.AuthKey,
			Dir:        filepath.Clean(stateDir),
			ControlURL: cfg.ControlURL,
			Ephemeral:  cfg.Ephemeral,
			Logf:       func(format string, args ...any) {}, // Suppress noisy internal tsnet debug logs by default
		}
		node.tsServer = tsServer
	}

	return node, nil
}

// Listen starts listening on the Tailscale network (or local fallback).
func (n *Node) Listen(ctx context.Context) (net.Listener, error) {
	if n.tsServer != nil {
		ln, err := n.tsServer.Listen("tcp", fmt.Sprintf(":%d", n.cfg.Port))
		if err != nil {
			return nil, fmt.Errorf("failed to listen on tailnet: %w", err)
		}
		n.listener = ln
		return ln, nil
	}

	// Local fallback TCP listener
	host := n.cfg.Host
	if host == "" {
		host = "127.0.0.1"
	}
	addr := net.JoinHostPort(host, fmt.Sprint(n.cfg.Port))
	// Short keepalives notice a vanished phone or peer in about 25 seconds, so
	// work tied to that connection stops instead of running on unobserved.
	lc := net.ListenConfig{KeepAliveConfig: net.KeepAliveConfig{Enable: true, Idle: 10 * time.Second, Interval: 5 * time.Second, Count: 3}}
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on local address %s: %w", addr, err)
	}
	n.listener = ln
	return ln, nil
}

// Dial connects to a remote peer machine over Tailscale (or local TCP).
func (n *Node) Dial(ctx context.Context, targetAddr string) (net.Conn, error) {
	if n.tsServer != nil {
		return n.tsServer.Dial(ctx, "tcp", targetAddr)
	}

	var d net.Dialer
	d.Timeout = 5 * time.Second
	return d.DialContext(ctx, "tcp", targetAddr)
}

// Close gracefully closes the listener and Tailscale node.
func (n *Node) Close() error {
	var err error
	if n.listener != nil {
		err = n.listener.Close()
	}
	if n.tsServer != nil {
		if tsErr := n.tsServer.Close(); tsErr != nil && err == nil {
			err = tsErr
		}
	}
	return err
}

// IsTailscaleEnabled returns true if the node is configured with an active Tailscale auth key.
func (n *Node) IsTailscaleEnabled() bool {
	return n.tsServer != nil
}
