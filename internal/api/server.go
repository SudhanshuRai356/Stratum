package api

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const allowedOrigin = "http://localhost:3000"

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return r.Header.Get("Origin") == allowedOrigin
	},
}

func handleWS(hub *Hub, w http.ResponseWriter, r *http.Request) {
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
