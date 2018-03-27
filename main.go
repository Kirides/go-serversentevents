package main

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/Kirides/go-serversentevents/ssebroker"
)

func main() {
	ctxt := context.Background()
	sseBroker := ssebroker.NewSseBroker()
	sseBroker.SetDebug(true)
	http.Handle("/sse-stream", sseBroker.HandleWithContext(ctxt))
	go sseBroker.ListenWithContext(ctxt)
	http.HandleFunc("/", indexHandler)
	go func() {
		for {
			time.Sleep(1 * time.Second)
			data := time.Now().Format("2006-01-02T15:04:05.999-07:00")
			sseBroker.SendEvent("1", "currentTime", data)
		}
	}()
	log.Fatal(http.ListenAndServe("127.0.0.1:5000", nil))
	ctxt.Done()
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	indexTemplate := template.New("index")
	index := template.Must(indexTemplate.Parse(htmlTemplate))
	w.Header().Add("Content-Type", "text/html")
	index.Execute(w, nil)
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
        var messages = document.getElementById("messages");
        var closeStream = document.getElementById("btn_close");
        var lblCurrentTime = document.getElementById("currentTime");

        var eventSource = new EventSource("http://127.0.0.1:5000/sse-stream");
        eventSource.onopen = function (x) {
            closeStream.onclick = function (x) {
                eventSource.close();
            };
        };
        eventSource.addEventListener("currentTime", function (timeEvent) {
            lblCurrentTime.innerText = "Current Time: " + timeEvent.data;
        })
    </script>
</body>

</html>`
