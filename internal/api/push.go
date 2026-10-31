package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// Phones get a notification when an agent needs the owner: a private reply,
// a mention of the owner, or a question, blocker, decision, plan, or done
// report in a group. Routine group chatter stays quiet. Notifications use Web
// Push, which needs the page served over HTTPS (mace-server start gives the
// tailnet HTTPS address) or from localhost. The service worker skips a
// notification while a MACE window is open and visible.

type pushKeys struct {
	Public  string `json:"public"`
	Private string `json:"private"`
}

type pushSubscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		Auth   string `json:"auth"`
		P256dh string `json:"p256dh"`
	} `json:"keys"`
}

type pushState struct {
	once sync.Once
	keys pushKeys
	err  error
	// send delivers one notification; tests replace it.
	send func(ctx context.Context, payload []byte, s pushSubscription, k pushKeys) (int, error)
}

func (h *APIHandler) pushKeys() (pushKeys, error) {
	h.push.once.Do(func() {
		if h.store == nil || h.store.Path() == ":memory:" {
			h.push.keys.Private, h.push.keys.Public, h.push.err = webpush.GenerateVAPIDKeys()
			return
		}
		path := filepath.Join(filepath.Dir(h.store.Path()), "push-keys.json")
		if b, err := os.ReadFile(path); err == nil && json.Unmarshal(b, &h.push.keys) == nil && h.push.keys.Public != "" {
			return
		}
		h.push.keys.Private, h.push.keys.Public, h.push.err = webpush.GenerateVAPIDKeys()
		if h.push.err == nil {
			b, _ := json.Marshal(h.push.keys)
			h.push.err = os.WriteFile(path, b, 0600)
		}
	})
	return h.push.keys, h.push.err
}

// pushKey serves GET /api/v1/push/key: the public key browsers subscribe with.
func (h *APIHandler) pushKey(w http.ResponseWriter, r *http.Request) {
	k, err := h.pushKeys()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, map[string]string{"key": k.Public})
}

// pushSubscribe serves POST /api/v1/push/subscribe and /unsubscribe.
func (h *APIHandler) pushSubscribe(w http.ResponseWriter, r *http.Request) {
	var s pushSubscription
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&s) != nil || !strings.HasPrefix(s.Endpoint, "https://") {
		http.Error(w, "invalid subscription", 400)
		return
	}
	id := hashHex(s.Endpoint)
	if strings.HasSuffix(r.URL.Path, "/unsubscribe") {
		h.store.DeleteDocument(r.Context(), "push", id)
		writeJSON(w, 200, map[string]bool{"subscribed": false})
		return
	}
	if s.Keys.Auth == "" || s.Keys.P256dh == "" {
		http.Error(w, "invalid subscription", 400)
		return
	}
	if err := h.store.PutDocument(r.Context(), "push", id, s); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 201, map[string]bool{"subscribed": true})
}

var attentionFlairs = map[string]bool{"question": true, "blocker": true, "decision": true, "plan": true, "done": true}

// needsOwner reports whether a new message should reach the owner's phone.
func (h *APIHandler) needsOwner(ctx context.Context, m Message) bool {
	if m.Sender == "you" || m.Sender == "mace" || m.Kind == "note" || m.Kind == "system" || m.Origin != "" {
		return false
	}
	var g Room
	if h.store.Document(ctx, "room", m.Room, &g) != nil {
		return true // a private chat
	}
	if g.Parent != "" {
		return false // side chats stay quiet
	}
	return attentionFlairs[m.Flair] || containsID(m.Mentions, "you")
}

// notify sends a new message to every subscribed device, dropping
// subscriptions the push service reports gone.
func (h *APIHandler) notify(m Message) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if !h.needsOwner(ctx, m) {
		return
	}
	raw, _ := h.store.Documents(ctx, "push")
	if len(raw) == 0 {
		return
	}
	keys, err := h.pushKeys()
	if err != nil {
		return
	}
	title := h.memberName(ctx, m.Sender)
	var g Room
	if h.store.Document(ctx, "room", m.Room, &g) == nil {
		title += " in " + g.Name
	}
	body := strings.Join(strings.Fields(m.Text), " ")
	if m.Flair != "" {
		body = "[" + m.Flair + "] " + body
	}
	payload, _ := json.Marshal(map[string]string{"title": limited(title, 120), "body": limited(body, 180), "room": m.Room, "tag": m.Room})
	send := h.push.send
	if send == nil {
		send = sendPush
	}
	for _, b := range raw {
		var s pushSubscription
		if json.Unmarshal(b, &s) != nil {
			continue
		}
		status, err := send(ctx, payload, s, keys)
		if status == http.StatusNotFound || status == http.StatusGone {
			h.store.DeleteDocument(ctx, "push", hashHex(s.Endpoint))
		} else if err != nil {
			log.Printf("[MACE] A notification was not delivered: %v", err)
		}
	}
}

func sendPush(ctx context.Context, payload []byte, s pushSubscription, k pushKeys) (int, error) {
	sub := &webpush.Subscription{Endpoint: s.Endpoint, Keys: webpush.Keys{Auth: s.Keys.Auth, P256dh: s.Keys.P256dh}}
	res, err := webpush.SendNotificationWithContext(ctx, payload, sub, &webpush.Options{
		Subscriber: "mace@localhost", VAPIDPublicKey: k.Public, VAPIDPrivateKey: k.Private, TTL: 3600, Urgency: webpush.UrgencyHigh,
	})
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return res.StatusCode, errors.New("the push service answered " + res.Status)
	}
	return res.StatusCode, nil
}
