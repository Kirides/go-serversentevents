package main

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/Kirides/go-serversentevents/ssebroker"
)

var srv = &http.Server{
	IdleTimeout:       60 * time.Second,
	ReadHeaderTimeout: 15 * time.Second,
	Addr:              "127.0.0.1:5000",
}

func main() {
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
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		indexTemplate := template.New("index")
		index := template.Must(indexTemplate.Parse(htmlTemplate))
		w.Header().Add("Content-Type", "text/html")
		index.Execute(w, nil)
	})
}

var htmlTemplate = `<!DOCTYPE html>
<html lang="en">

<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <meta http-equiv="X-UA-Compatible" content="ie=edge">
    <title>Document</title>
</head>

<body>
    <button id=btn_close>close Stream</button>
    <label id=currentTime />
    <ul id=messages />
    <script>
        if (window.EventSource !== undefined) {
            initSSE();
        }

        function initSSE() {
            var messages = document.getElementById("messages");
            var closeStream = document.getElementById("btn_close");
            var lblCurrentTime = document.getElementById("currentTime");

            var eventSource = new EventSource("/sse-stream");
            eventSource.onopen = function (x) {
                closeStream.onclick = function (x) {
                    eventSource.close();
                };
            };
            eventSource.addEventListener("currentTime", function (timeEvent) {
                lblCurrentTime.innerText = "Current Time: " + timeEvent.data;
            })
        }
    </script>
</body>

</html>`
