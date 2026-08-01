package api

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"speedtest/internal/engine"
)

// Client represents a connected SSE client.
type Client struct {
	ch  chan []byte
	out chan struct{}
}

// Broadcaster manages connected SSE clients and distributes events.
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

// Broadcast sends a ProgressEvent to all connected clients.
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

// SSEHandler — Server-Sent Events with heartbeat keepalive.
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

	// Heartbeat ticker — keeps proxies from dropping idle connections
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

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
		case <-heartbeat.C:
			w.Write([]byte(": keepalive\n\n"))
			flusher.Flush()
		}
	}
}
