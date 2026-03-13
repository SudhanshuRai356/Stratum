package api

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const allowedOrigin = "http://localhost:3000"

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

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		return origin == allowedOrigin || origin == ""
	},
}

func handleWS(hub *Hub, w http.ResponseWriter, r *http.Request) {
	// Verify per-session token
	if r.URL.Query().Get("token") != apiToken {
		http.Error(w, "unauthorized", http.StatusForbidden)
		return
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
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWS(hub, w, r)
	})

	srv := &http.Server{
		Addr:    "127.0.0.1:8080",
		Handler: mux,
	}

	log.Printf("api: listening on %s", srv.Addr)
	return srv.ListenAndServe()
}
