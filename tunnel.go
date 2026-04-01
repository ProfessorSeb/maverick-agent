package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// Message is the WebSocket message envelope (mirrors server protocol).
type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// ConfigPayload is the configuration pushed from the control plane.
type ConfigPayload struct {
	Listeners []ListenerConfig `json:"listeners"`
	Routes    []RouteConfig    `json:"routes"`
	Backends  []BackendConfig  `json:"backends"`
}

// ListenerConfig represents a listener from the control plane.
type ListenerConfig struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Address     string `json:"address"`
	IsEnabled   bool   `json:"is_enabled"`
}

// RouteConfig represents a route from the control plane.
type RouteConfig struct {
	ID         string   `json:"id"`
	ListenerID string   `json:"listener_id"`
	Name       string   `json:"name"`
	PathValue  string   `json:"path_value"`
	BackendIDs []string `json:"backend_ids,omitempty"`
	IsEnabled  bool     `json:"is_enabled"`
}

// BackendConfig represents a backend from the control plane.
type BackendConfig struct {
	ID           string `json:"id"`
	RouteID      string `json:"route_id"`
	Name         string `json:"name"`
	BackendType  string `json:"backend_type"`
	ProviderType string `json:"provider_type,omitempty"`
	Model        string `json:"model,omitempty"`
	APIKeyRef    string `json:"api_key_ref,omitempty"`
	Weight       int    `json:"weight"`
	IsEnabled    bool   `json:"is_enabled"`
}

// TunnelClient manages the WebSocket connection to the control plane.
type TunnelClient struct {
	URL       string
	Token     string
	ConfigDir string
	GWPath    string
	Proc      *ProcessManager
}

// Run connects to the control plane and handles reconnection with exponential backoff.
func (t *TunnelClient) Run(ctx context.Context) {
	backoff := time.Second
	maxBackoff := 60 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := t.connect(ctx)
		if err != nil {
			log.Printf("tunnel: connection error: %v", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		log.Printf("tunnel: reconnecting in %v...", backoff)
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// connect establishes a WebSocket connection and runs read/write loops.
func (t *TunnelClient) connect(ctx context.Context) error {
	header := http.Header{}
	header.Set("Authorization", "Bearer "+t.Token)

	// Enforce secure WebSocket in production
	if !strings.HasPrefix(t.URL, "wss://") && !strings.HasPrefix(t.URL, "ws://localhost") && !strings.HasPrefix(t.URL, "ws://127.0.0.1") {
		log.Printf("WARNING: tunnel connection is not using TLS (wss://). This is insecure.")
	}

	log.Printf("tunnel: connecting to %s", t.URL)
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	ws, _, err := dialer.DialContext(ctx, t.URL, header)
	if err != nil {
		return err
	}
	defer ws.Close()

	log.Println("tunnel: connected to control plane")

	// Send ready message
	readyMsg, _ := json.Marshal(Message{Type: "ready", Payload: json.RawMessage(`{}`)})
	if err := ws.WriteMessage(websocket.TextMessage, readyMsg); err != nil {
		return err
	}

	// Start heartbeat sender
	heartbeatDone := make(chan struct{})
	go t.heartbeatLoop(ctx, ws, heartbeatDone)

	// Read loop
	err = t.readLoop(ctx, ws)

	close(heartbeatDone)
	return err
}

// readLoop processes incoming messages from the control plane.
func (t *TunnelClient) readLoop(ctx context.Context, ws *websocket.Conn) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		ws.SetReadDeadline(time.Now().Add(90 * time.Second))
		_, data, err := ws.ReadMessage()
		if err != nil {
			return err
		}

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("tunnel: received invalid message: %v", err)
			continue
		}

		switch msg.Type {
		case "config_push", "config_update":
			t.handleConfig(msg.Payload)
		case "error":
			log.Printf("tunnel: server error: %s", string(msg.Payload))
		default:
			log.Printf("tunnel: unknown message type: %s", msg.Type)
		}
	}
}

// heartbeatLoop sends periodic heartbeat messages.
func (t *TunnelClient) heartbeatLoop(ctx context.Context, ws *websocket.Conn, done chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Send initial heartbeat immediately
	t.sendHeartbeat(ws)

	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			t.sendHeartbeat(ws)
		}
	}
}

// sendHeartbeat collects system info and sends a heartbeat message.
func (t *TunnelClient) sendHeartbeat(ws *websocket.Conn) {
	hb := CollectHeartbeat(t.Proc)

	payload, err := json.Marshal(hb)
	if err != nil {
		log.Printf("tunnel: failed to marshal heartbeat: %v", err)
		return
	}

	msg := Message{
		Type:    "heartbeat",
		Payload: payload,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := ws.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("tunnel: failed to send heartbeat: %v", err)
	}
}

// handleConfig processes a config push/update from the server.
func (t *TunnelClient) handleConfig(payload json.RawMessage) {
	var cfg ConfigPayload
	if err := json.Unmarshal(payload, &cfg); err != nil {
		log.Printf("tunnel: failed to parse config: %v", err)
		return
	}

	log.Printf("tunnel: received config (%d listeners, %d routes, %d backends)",
		len(cfg.Listeners), len(cfg.Routes), len(cfg.Backends))

	configPath := filepath.Join(t.ConfigDir, "agentgateway.yaml")
	if err := WriteConfig(t.ConfigDir, cfg); err != nil {
		log.Printf("tunnel: failed to write config: %v", err)
		return
	}

	// Start or restart agentgateway
	gwPath := t.GWPath
	if gwPath == "" {
		gwPath = "agentgateway" // rely on PATH
	}

	if t.Proc.IsRunning() {
		log.Println("tunnel: restarting agentgateway with new config")
		t.Proc.Restart(gwPath, configPath)
	} else {
		log.Println("tunnel: starting agentgateway")
		t.Proc.Start(gwPath, configPath)
	}
}
