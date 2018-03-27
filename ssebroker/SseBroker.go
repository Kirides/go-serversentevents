package ssebroker

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// SseBroker ...
type SseBroker struct {
	clients         map[chan []byte]bool
	onMessage       chan []byte
	onNewClient     chan chan []byte
	onClientClosing chan chan []byte
	debug           bool
}

// NewSseBroker ...
func NewSseBroker() *SseBroker {
	return &SseBroker{
		onMessage:       make(chan []byte, 1),
		onNewClient:     make(chan chan []byte),
		onClientClosing: make(chan chan []byte),
		clients:         make(map[chan []byte]bool),
	}
}

// SetDebug enables debugging logs
func (b *SseBroker) SetDebug(v bool) {
	b.debug = v
}

// ListenWithContext ...
func (b *SseBroker) ListenWithContext(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case c := <-b.onNewClient:
			b.clients[c] = true
			if b.debug {
				log.Printf("New Client for SSE. Total %d\n", len(b.clients))
			}
			break
		case c := <-b.onClientClosing:
			delete(b.clients, c)
			if b.debug {
				log.Printf("SSE-Client left. Total %d\n", len(b.clients))
			}
			break
		case c := <-b.onMessage:
			for client := range b.clients {
				client <- c
			}
			break
		}
	}
}

// SendEvent ...
func (b SseBroker) SendEvent(eventID string, eventName string, value []byte) {
	b.onMessage <- []byte(fmt.Sprintf("id: %s\nevent: %s\ndata: %s\n\n", eventID, eventName, value))
}

// SendMessage ...
func (b SseBroker) SendMessage(eventID string, eventName string, value []byte) {
	b.onMessage <- []byte(fmt.Sprintf("data: %s\n\n", value))
}

// HandleAndListenWithContext Starts a SSE-Broker and handles requests to the bound path
func (b *SseBroker) HandleAndListenWithContext(ctx context.Context) http.Handler {
	go b.ListenWithContext(ctx)
	return b.HandleWithContext(ctx)
}

// HandleWithContext Handles request to a listening Broker
func (b *SseBroker) HandleWithContext(ctx context.Context) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "text/event-stream")
		w.Header().Add("Cache-Control", "no-cache, no-store, must-revalidate, proxy-revalidate")
		w.Header().Add("Connection", "keep-alive")

		origin := r.Header.Get("Origin")
		if origin == "null" {
			origin = "*"
		}
		w.Header().Add("Transfer-Encoding", "chunked")
		if origin != "" {
			w.Header().Add("Access-Control-Allow-Origin", origin)
		}
		w.Header().Add("Access-Control-Allow-Headers", "*")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		// isLegacy := r.Header.Get("X-Requested-With") == "XMLHttpRequest"
		isLegacy := strings.Contains(r.Header.Get("User-Agent"), "Edge")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Server-Sent Events not supportet!", http.StatusInternalServerError)
			return
		}
		messageChan := make(chan []byte)
		b.onNewClient <- messageChan
		closeClientConnection := func() { b.onClientClosing <- messageChan }
		defer closeClientConnection()

		// Listen to connection close and un-register messageChan
		closeNotify := w.(http.CloseNotifier).CloseNotify()

		// block waiting for messages broadcast on this connection's messageChan
		for {
			select {
			case msg := <-messageChan:
				_, err := w.Write(msg)
				if err != nil {
					if b.debug {
						log.Println("Error writing data to SSE-Client")
					}
					return
				}
				// Flush the data immediatly instead of buffering it for later.
				flusher.Flush()
				if isLegacy {
					return
				}
				break
			case <-closeNotify:
			case <-ctx.Done():
				return
			}
		}
	})
}
