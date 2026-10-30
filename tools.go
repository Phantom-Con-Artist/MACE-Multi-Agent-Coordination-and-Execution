//go:build tools

package tools

import (
	_ "github.com/go-chi/chi/v5"
	_ "github.com/gorilla/websocket"
	_ "github.com/libp2p/go-libp2p"
	_ "golang.org/x/sync/errgroup"
	_ "modernc.org/sqlite"
	_ "tailscale.com/tsnet"
)
