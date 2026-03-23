package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/dell/IRC-Reference-Tools/internal/logging"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/auth"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/config"
	"github.com/gin-gonic/gin"
)

// SSEEvent is a typed Server-Sent Event ready to be written to the wire.
type SSEEvent struct {
	Event string `json:"event"` // SSE "event:" field
	Data  string `json:"data"`  // SSE "data:" field (JSON string)
}

// SSEHub fans out SSE events to all connected browser clients.
// It is safe for concurrent use.
type SSEHub struct {
	mu      sync.RWMutex
	clients map[chan SSEEvent]struct{}
}

// NewSSEHub creates and returns a new SSE hub.
func NewSSEHub() *SSEHub {
	return &SSEHub{
		clients: make(map[chan SSEEvent]struct{}),
	}
}

// Register adds a new client channel and returns it.
func (h *SSEHub) Register() chan SSEEvent {
	ch := make(chan SSEEvent, 64)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// Unregister removes and closes a client channel.
func (h *SSEHub) Unregister(ch chan SSEEvent) {
	h.mu.Lock()
	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
	h.mu.Unlock()
}

// Broadcast sends an event to every connected client.
// Slow clients whose buffers are full are skipped (non-blocking send).
func (h *SSEHub) Broadcast(event SSEEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	logging.Info("SSE hub broadcasting", "event", event.Event, "clients", len(h.clients))
	for ch := range h.clients {
		select {
		case ch <- event:
		default:
			logging.Warn("SSE client buffer full, dropping event", "event", event.Event)
		}
	}
}

// ClientCount returns the number of connected SSE clients.
func (h *SSEHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// StartHeartbeat sends a heartbeat comment every interval to keep
// connections alive through proxies and load balancers.
func (h *SSEHub) StartHeartbeat(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			h.Broadcast(SSEEvent{Event: "heartbeat", Data: "{}"})
		}
	}()
}

// buildSnapshot queries the current full state and returns a snapshot SSE event.
func buildSnapshot(s *config.SystemHandler) SSEEvent {
	services := s.AuthClient.GetAllServices()
	valvestatus := s.AuthClient.GetValveStatus()

	type ircSnapshot struct {
		IP          string `json:"ip"`
		State       string `json:"state"`
		Valve1State string `json:"valve1state"`
		Valve2State string `json:"valve2state"`
	}

	var ircs []ircSnapshot
	for _, svc := range services {
		irc := ircSnapshot{
			IP:    svc.Ip,
			State: svc.State,
		}
		for _, v := range valvestatus {
			if v.Ip == svc.Ip {
				irc.Valve1State = v.VState1
				irc.Valve2State = v.VState2
			}
		}
		ircs = append(ircs, irc)
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"ircs": ircs,
	})
	return SSEEvent{Event: "snapshot", Data: string(payload)}
}

// sseHandler is the Gin handler for GET /api/v1/events.
// It sends an initial snapshot, then streams live delta events.
func sseHandler(c *gin.Context, hub *SSEHub, s *config.SystemHandler) {
	// SSE response headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // disable nginx buffering

	// Register with the hub
	clientCh := hub.Register()
	defer hub.Unregister(clientCh)
	logging.Info("SSE client connected", "clients", hub.ClientCount())

	// Send initial snapshot so the browser has full state
	snapshot := buildSnapshot(s)
	logging.Debug("SSE sending snapshot to new client", "data", snapshot.Data)
	writeSSE(c.Writer, snapshot)
	c.Writer.Flush()

	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			logging.Info("SSE client disconnected", "clients", hub.ClientCount()-1)
			// Client disconnected
			return
		case event, ok := <-clientCh:
			if !ok {
				return
			}
			writeSSE(c.Writer, event)
			c.Writer.Flush()
		}
	}
}

// writeSSE writes a single SSE event to the writer.
func writeSSE(w io.Writer, event SSEEvent) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Event, event.Data)
}

// notificationToSSEEvent converts an auth.UINotification into an SSEEvent.
func notificationToSSEEvent(n auth.UINotification) SSEEvent {
	data, err := json.Marshal(n.Data)
	if err != nil {
		logging.Error("Failed to marshal SSE event data", "err", err)
		data = []byte("{}")
	}
	return SSEEvent{
		Event: n.Event,
		Data:  string(data),
	}
}
