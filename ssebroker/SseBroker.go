package ssebroker

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

var (
	idBytes      = []byte("id: ")
	eventBytes   = []byte("event: ")
	dataBytes    = []byte("data: ")
	newLineBytes = []byte("\n")
)

// SseBroker ...
type SseBroker struct {
	clients         map[chan []byte]bool
	onMessage       chan []byte
	onNewClient     chan chan []byte
	onClientClosing chan chan []byte
	logger          *log.Logger
	buf             *bytes.Buffer
	mtx             *sync.Mutex
}

// NewSseBroker ...
func NewSseBroker(logger *log.Logger) *SseBroker {
	if logger == nil {
		logger = log.New(os.Stdout, "", log.Flags())
	}
	return &SseBroker{
		onMessage:       make(chan []byte, 1),
		onNewClient:     make(chan chan []byte),
		onClientClosing: make(chan chan []byte),
		clients:         make(map[chan []byte]bool),
		logger:          logger,
		buf:             new(bytes.Buffer),
		mtx:             new(sync.Mutex),
	}
}

// ListenWithContext ...
func (b *SseBroker) ListenWithContext(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case c := <-b.onNewClient:
			b.clients[c] = true
			b.logger.Printf("New Client for SSE. Total %d\n", len(b.clients))
			break
		case c := <-b.onClientClosing:
			delete(b.clients, c)
			b.logger.Printf("SSE-Client left. Total %d\n", len(b.clients))
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
func (b *SseBroker) SendEvent(eventID, eventName, value string) {
	b.mtx.Lock()
	b.buf.Reset()

	b.writeBuffer(idBytes, eventID)
	b.writeBuffer(eventBytes, eventName)
	b.writeBuffer(dataBytes, value)

	b.buf.Write(newLineBytes)

	b.onMessage <- b.buf.Bytes()
	b.mtx.Unlock()
}

// SendMessage ...
func (b *SseBroker) SendMessage(value string) {
	b.mtx.Lock()
	b.buf.Reset()

	b.writeBuffer(dataBytes, value)
	b.buf.Write(newLineBytes)

	b.onMessage <- b.buf.Bytes()
	b.mtx.Unlock()
}

func (b *SseBroker) writeBuffer(key []byte, value string) {
	b.buf.Write(key)
	b.buf.WriteString(value)
	b.buf.Write(newLineBytes)
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
					b.logger.Printf("Error writing data to SSE-Client")
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
