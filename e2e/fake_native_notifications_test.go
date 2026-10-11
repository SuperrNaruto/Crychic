package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const nativePrefix = "/api/v1/plugin/Crychic/"

type nativeRecipient struct {
	Kind  string `json:"kind"`
	Chat  string `json:"chat_id"`
	User  string `json:"user_id,omitempty"`
	Topic int    `json:"topic_id,omitempty"`
}

type nativeMessage struct {
	Sequence  uint64          `json:"sequence"`
	ID        string          `json:"id"`
	Platform  string          `json:"platform"`
	Recipient nativeRecipient `json:"recipient"`
	Title     string          `json:"title"`
	Text      string          `json:"text"`
	Image     string          `json:"image,omitempty"`
	Link      string          `json:"link,omitempty"`
}

type nativeMP struct {
	mu        sync.Mutex
	head      uint64
	acked     uint64
	items     []nativeMessage
	wake      chan struct{}
	baseline  chan struct{}
	consumer  string
	active    bool
	malformed bool
}

func newNativeMP() *nativeMP {
	return &nativeMP{wake: make(chan struct{}), baseline: make(chan struct{}), active: true}
}

func (n *nativeMP) metadata() map[string]any {
	return map[string]any{"protocol": 1, "channel_id": "test-channel", "stream_id": "test-stream", "head": n.head, "active": n.active}
}

func (n *nativeMP) publish(message nativeMessage) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.head++
	message.Sequence, message.ID, message.Platform = n.head, fmt.Sprintf("native-%d", n.head), "telegram"
	n.items = append(n.items, message)
	close(n.wake)
	n.wake = make(chan struct{})
}

func (n *nativeMP) serve(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, nativePrefix) {
		return false
	}
	switch r.URL.Path {
	case nativePrefix + "status":
		n.mu.Lock()
		defer n.mu.Unlock()
		n.write(w, n.metadata())
	case nativePrefix + "notifications":
		n.read(w, r)
	case nativePrefix + "ack":
		n.ack(w, r)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
	return true
}

func (n *nativeMP) read(w http.ResponseWriter, r *http.Request) {
	after, err := strconv.ParseUint(r.URL.Query().Get("after"), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	n.mu.Lock()
	wake := n.wake
	ready := n.head > after || !n.active
	n.mu.Unlock()
	if !ready {
		select {
		case <-wake:
		case <-r.Context().Done():
			return
		case <-time.After(actionTimeout):
		}
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	data := n.metadata()
	items := []nativeMessage{}
	for _, item := range n.items {
		if item.Sequence > after {
			items = append(items, item)
		}
	}
	data["messages"] = items
	if n.malformed {
		data["messages"] = nil
	}
	n.write(w, data)
}

func (n *nativeMP) ack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var input struct {
		Consumer string `json:"consumer_id"`
		Sequence uint64 `json:"sequence"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.Consumer == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.consumer != "" && n.consumer != input.Consumer {
		w.WriteHeader(http.StatusConflict)
		return
	}
	if input.Sequence > n.head {
		w.WriteHeader(http.StatusConflict)
		return
	}
	n.consumer, n.acked = input.Consumer, max(n.acked, input.Sequence)
	select {
	case <-n.baseline:
	default:
		close(n.baseline)
	}
	n.write(w, map[string]any{"sequence": n.acked})
}

func (n *nativeMP) write(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
}
