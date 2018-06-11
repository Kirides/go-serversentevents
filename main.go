package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/Kirides/go-serversentevents/ssebroker"
)

var srv = &http.Server{
	IdleTimeout:       60 * time.Second,
	ReadHeaderTimeout: 15 * time.Second,
	Addr:              "127.0.0.1:5000",
}
var exePath string

func main() {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	exePath = wd

	ctxt, cancel := context.WithCancel(context.Background())
	sseBroker := ssebroker.NewSseBroker(nil)
	mux := http.NewServeMux()
	srv.Handler = mux

	mux.Handle("/sse-stream", sseBroker.HandleAndListenWithContext(ctxt))
	mux.Handle("/", http.TimeoutHandler(indexHandler(), time.Second*10, ""))
	go func() {
		for {
			time.Sleep(1 * time.Second)
			data := time.Now().Format("2006-01-02T15:04:05.999-07:00")
			sseBroker.SendEvent("1", "currentTime", data)
		}
	}()
	go func() {
		log.Printf("Server running on %s", srv.Addr)
		log.Println(srv.ListenAndServe())
	}()
	handleShutdown(ctxt, cancel)
}

func handleShutdown(ctx context.Context, cancel context.CancelFunc) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	log.Println("Received interrupt. Shutting down server.")
	cancel()
	srv.Shutdown(ctx)
}

func indexHandler() http.Handler {
	indexPath := filepath.Join(exePath, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		http.ServeFile(w, r, indexPath)
	})
}
