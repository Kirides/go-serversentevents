package ssebroker

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
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
	return b.handleWithContextAndCompression(ctx, "", nil)
}

// HandleWithContextAndGzip Handles request to a listening Broker
func (b *SseBroker) HandleWithContextAndGzip(ctx context.Context) http.Handler {
	return b.handleWithContextAndCompression(ctx, "gzip", func(w io.Writer) io.WriteCloser {
		return gzip.NewWriter(w)
	})
}

// HandleWithContext Handles request to a listening Broker
func (b *SseBroker) handleWithContextAndCompression(ctx context.Context, contentEncoding string, compressFn func(io.Writer) io.WriteCloser) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "text/event-stream")
		w.Header().Add("Cache-Control", "no-cache, no-store, must-revalidate, proxy-revalidate")
		w.Header().Add("Connection", "keep-alive")
		if contentEncoding != "" && compressFn != nil && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Add("Content-Encoding", contentEncoding)
		} else {
			compressFn = nil
		}
		w.Header().Add("Transfer-Encoding", "chunked")
		w.Header().Add("Access-Control-Allow-Origin", r.Header.Get("Origin"))
		w.Header().Add("Access-Control-Allow-Headers", "*")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		// isLegacy := r.Header.Get("X-Requested-With") == "XMLHttpRequest"
		isLegacy := strings.Contains(r.Header.Get("User-Agent"), "Edge") || (strings.Contains(r.Header.Get("User-Agent"), "Android") && strings.Contains(r.Header.Get("User-Agent"), "Edge/"))
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
				if compressFn != nil {
					cp := compressFn(w)
					_, err := cp.Write(msg)
					if err != nil {
						log.Println("Error writing data to SSE-Client")
						cp.Close()
						return
					}
					cp.Close()
				} else {
					_, err := w.Write(msg)
					if err != nil {
						log.Println("Error writing data to SSE-Client")
						return
					}
				}

				// Flush the data immediatly instead of buffering it for later.
				flusher.Flush()
				if isLegacy {
					return
				}
				break
			case <-ctx.Done():
				closeClientConnection()
				return
			}
		}
	})
}
