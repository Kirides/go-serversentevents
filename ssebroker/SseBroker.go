package ssebroker

import (
	"context"
	"fmt"
	"log"
	"net/http"
)

// SseBroker ...
type SseBroker struct {
	clients         map[chan []byte]bool
	onMessage       chan []byte
	onNewClient     chan chan []byte
	onClientClosing chan chan []byte
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

// ListenWithContext ...
func (b *SseBroker) ListenWithContext(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			break
		case c := <-b.onNewClient:
			b.clients[c] = true
			log.Printf("New Client for SSE. Total %d\n", len(b.clients))
			break
		case c := <-b.onClientClosing:
			delete(b.clients, c)
			log.Printf("SSE-Client left. Total %d\n", len(b.clients))
			break
		case c := <-b.onMessage:
			for client := range b.clients {
				client <- c
			}
			break
		default:
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

// HandleAndListen ...
func (b *SseBroker) HandleAndListen() http.Handler {
	ctx := context.Background()
	go b.ListenWithContext(ctx)
	return b.HandleWithContext(ctx)
}

// Handle ...
func (b *SseBroker) Handle() http.Handler {
	return b.HandleWithContext(context.Background())
}

// HandleWithContext Handles request to a listening Broker
func (b *SseBroker) HandleWithContext(ctx context.Context) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "text/event-stream")
		w.Header().Add("Cache-Control", "no-cache")
		w.Header().Add("Connection", "keep-alive")
		w.Header().Add("Access-Control-Allow-Origin", r.Header.Get("Origin"))
		if r.Method == http.MethodOptions {
			return
		}

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

		go func() {
			select {
			case <-closeNotify:
				closeClientConnection()
				break
			case <-ctx.Done():
				closeClientConnection()
				return
			}
		}()

		// block waiting for messages broadcast on this connection's messageChan
		for {
			select {
			case msg := <-messageChan:
				_, err := w.Write(msg)
				if err != nil {
					log.Println("Error writing data to SSE-Client")
					return
				}
				// Flush the data immediatly instead of buffering it for later.
				flusher.Flush()
				break
			case <-ctx.Done():
				closeClientConnection()
				return
			}
		}
	})
}
