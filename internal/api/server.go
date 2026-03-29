package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var defaultAllowedOrigins = []string{
	"http://127.0.0.1:3000",
	"http://127.0.0.1:3001",
	"http://127.0.0.1:1420",
	"http://localhost:3000",
	"http://localhost:3001",
	"http://localhost:1420",
	"https://tauri.localhost",
	"http://tauri.localhost",
	"tauri://localhost",
}

// apiToken is a per-session secret printed to stdout on startup.
// Clients must pass it as ?token=<value> to authenticate.
var apiToken string

func init() {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate API token: " + err.Error())
	}
	apiToken = hex.EncodeToString(b)
}

// GetAPIToken returns the session token for display to the user.
func GetAPIToken() string {
	return apiToken
}

type ControlPayload struct {
	Action   string `json:"action"`
	InfoHash string `json:"info_hash,omitempty"`
}

type OpenPayload struct {
	InfoHash string `json:"info_hash,omitempty"`
}

type StartPayload struct {
	Source        string `json:"source,omitempty"`
	TorrentBase64 string `json:"torrent_base64,omitempty"`
	Filename      string `json:"filename,omitempty"`
}
type ServerConfig struct {
	Addr          string
	Hub           *Hub
	AllowedOrigin []string
	GetState      func() []TorrentState
	HandleControl func(payload ControlPayload) (string, error)
	HandleOpen    func(payload OpenPayload) (string, error)
	HandleStart   func(payload StartPayload) (string, error)
}

func normalizeAllowedOrigins(origins []string) map[string]struct{} {
	if len(origins) == 0 {
		origins = defaultAllowedOrigins
	}
	out := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		trimmed := strings.TrimSpace(origin)
		if trimmed == "" {
			continue
		}
		out[trimmed] = struct{}{}
	}
	return out
}

func isAllowedOrigin(origin string, allowed map[string]struct{}) bool {
	if origin == "" {
		return true
	}
	_, ok := allowed[origin]
	return ok
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func withCORS(next http.Handler, allowed map[string]struct{}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if isAllowedOrigin(origin, allowed) && origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func handleWS(hub *Hub, allowed map[string]struct{}, w http.ResponseWriter, r *http.Request) {
	if hub == nil {
		http.Error(w, "hub unavailable", http.StatusServiceUnavailable)
		return
	}

	// Verify per-session token
	if r.URL.Query().Get("token") != apiToken {
		http.Error(w, "unauthorized", http.StatusForbidden)
		return
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return isAllowedOrigin(r.Header.Get("Origin"), allowed)
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade failed: %v", err)
		return
	}

	hub.Register <- conn

	defer func() {
		hub.Unregister <- conn
	}()

	// Read pump: required so the websocket library processes control frames
	// (ping/pong/close). We discard all application-level data for now.
	conn.SetReadLimit(512)
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

func ListenAndServe(hub *Hub) error {
	return ListenAndServeWithConfig(ServerConfig{Hub: hub, Addr: "127.0.0.1:8080"})
}

func ListenAndServeWithConfig(cfg ServerConfig) error {
	if cfg.Hub == nil {
		return errors.New("api: hub is required")
	}
	if strings.TrimSpace(cfg.Addr) == "" {
		cfg.Addr = "127.0.0.1:8080"
	}

	allowed := normalizeAllowedOrigins(cfg.AllowedOrigin)
	wsURL := "ws://" + cfg.Addr + "/ws"

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":   true,
			"time": time.Now().UTC().Format(time.RFC3339),
		})
	})

	mux.HandleFunc("/api/ws-auth", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"token":  apiToken,
			"ws_url": wsURL,
		})
	})

	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		states := []TorrentState{}
		if cfg.GetState != nil {
			states = cfg.GetState()
		}
		writeJSON(w, http.StatusOK, map[string]any{"torrents": states})
	})

	mux.HandleFunc("/api/control", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if cfg.HandleControl == nil {
			http.Error(w, "control not configured", http.StatusNotImplemented)
			return
		}

		defer r.Body.Close()
		var payload ControlPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}

		message, err := cfg.HandleControl(payload)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"status":  "error",
				"message": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"message": message,
		})
	})

	mux.HandleFunc("/api/open", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if cfg.HandleOpen == nil {
			http.Error(w, "open not configured", http.StatusNotImplemented)
			return
		}

		defer r.Body.Close()
		var payload OpenPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}

		message, err := cfg.HandleOpen(payload)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"status":  "error",
				"message": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"message": message,
		})
	})

	mux.HandleFunc("/api/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if cfg.HandleStart == nil {
			http.Error(w, "start not configured", http.StatusNotImplemented)
			return
		}

		defer r.Body.Close()
		var payload StartPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}

		message, err := cfg.HandleStart(payload)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"status":  "error",
				"message": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"message": message,
		})
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWS(cfg.Hub, allowed, w, r)
	})

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: withCORS(mux, allowed),
	}

	log.Printf("api: listening on %s", srv.Addr)
	log.Printf("api: websocket endpoint %s", wsURL)
	return srv.ListenAndServe()
}
