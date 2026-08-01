package api

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"speedtest/internal/engine"
)

// Client represents a connected WebSocket client.
type Client struct {
	ch  chan []byte
	out chan struct{}
}

// Broadcaster managed verbunden WebSocket-Clients und verteilt Events.
type Broadcaster struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		clients: make(map[*Client]struct{}),
	}
}

func (b *Broadcaster) Register() *Client {
	c := &Client{
		ch:  make(chan []byte, 64),
		out: make(chan struct{}),
	}
	b.mu.Lock()
	b.clients[c] = struct{}{}
	b.mu.Unlock()
	return c
}

func (b *Broadcaster) Unregister(c *Client) {
	b.mu.Lock()
	delete(b.clients, c)
	b.mu.Unlock()
	close(c.out)
}

// Broadcast sendet ein ProgressEvent an alle verbunden Clients.
func (b *Broadcaster) Broadcast(ev engine.ProgressEvent) {
	data, err := json.Marshal(ev)
	if err != nil {
		log.Printf("Failed to marshal event: %v", err)
		return
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for c := range b.clients {
		select {
		case c.ch <- data:
		default:
			// Client buffer full — skip (don't block)
		}
	}
}

// === WebSocket Handler ===

// WebSocketHandler handled /ws — nutzt net/http WebSocket Upgrade via golang.org/x/net/websocket?
// Nein — wir nutzen das Standard net/http Hijack Pattern oder nhooyr.io/websocket.
// Fürs erste nutzen wir die einfache Standard-Bibliothek ohne externe WS-Lib.
// Da Go stdlib keinen WS hat, nutzen wir nhooyr.io/websocket.
// Der Handler wird in Task 6 finalisiert, hier erstmal SSE als simpler Fallback.

// SSEHandler — Server-Sent Events als einfache Alternative zu WebSocket.
// Browser EventSource API ist nativ und braucht keine externe Lib.
func (s *AppState) SSEHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	client := s.Broadcaster.Register()
	defer s.Broadcaster.Unregister(client)

	notify := r.Context().Done()

	// Send initial connection confirmation
	w.Write([]byte(": connected\n\n"))
	flusher.Flush()

	for {
		select {
		case <-notify:
			return
		case data := <-client.ch:
			w.Write([]byte("data: "))
			w.Write(data)
			w.Write([]byte("\n\n"))
			flusher.Flush()
		case <-client.out:
			return
		}
	}
}
