package api

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"mace/internal/agent"
	"mace/pkg/protocol"
	"net/http"
	"time"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return r.Header.Get("Origin") == "" || r.Header.Get("Origin") == "http://"+r.Host || r.Header.Get("Origin") == "https://"+r.Host
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type wsClientMessage struct {
	Action string `json:"action"` // "input", "stop"
	Data   string `json:"data,omitempty"`
	Signal string `json:"signal,omitempty"`
}

func handleAgentWebSocket(supervisor *agent.Supervisor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentID := routeParam(r, "id")
		if agentID == "" {
			http.Error(w, "missing agent id", http.StatusBadRequest)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		eventCh, unsubscribe := supervisor.Subscribe(agentID)
		defer unsubscribe()

		done := make(chan struct{})

		// Reader pump (processes incoming input or stop commands from client)
		go func() {
			defer close(done)
			for {
				_, msgBytes, err := conn.ReadMessage()
				if err != nil {
					return
				}

				var msg wsClientMessage
				if err := json.Unmarshal(msgBytes, &msg); err == nil {
					switch msg.Action {
					case "input":
						_ = supervisor.SendInput(agentID, msg.Data)
					case "stop":
						_ = supervisor.StopAgent(agentID, msg.Signal, 2*time.Second)
					}
				}
			}
		}()

		// Writer pump (forwards agent stdout/stderr/status events to websocket client)
		for {
			select {
			case <-done:
				return
			case event, ok := <-eventCh:
				if !ok {
					return
				}
				_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if err := conn.WriteJSON(event); err != nil {
					return
				}
				if event.Type == protocol.EventExit {
					// Give client time to receive final frame
					time.Sleep(50 * time.Millisecond)
					return
				}
			}
		}
	}
}
