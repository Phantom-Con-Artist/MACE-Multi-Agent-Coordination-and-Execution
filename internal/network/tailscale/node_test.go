package tailscale

import (
	"context"
	"io"
	"testing"
	"time"
)

func TestNode_LocalFallback(t *testing.T) {
	node, err := NewNode(Config{
		Port: 0,
	})
	if err != nil {
		t.Fatalf("NewNode failed: %v", err)
	}
	defer node.Close()

	if node.IsTailscaleEnabled() {
		t.Error("expected Tailscale to be disabled without AuthKey")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ln, err := node.Listen(ctx)
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}

	// Echo server goroutine
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(conn, conn)
	}()

	// Dial via node
	clientConn, err := node.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer clientConn.Close()

	msg := "hello mace node"
	if _, err := clientConn.Write([]byte(msg)); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(clientConn, buf); err != nil {
		t.Fatalf("ReadFull failed: %v", err)
	}

	if string(buf) != msg {
		t.Errorf("expected echoed msg %s, got %s", msg, string(buf))
	}
}
