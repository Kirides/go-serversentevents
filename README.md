# go-serversentevents
Sample application that shows usage of ServerSentEvents and supporting gzip-compression.

## Basic Usage
```golang
sseBroker := ssebroker.NewSseBroker()
http.Handle("/sse-stream", sseBroker.HandleAndListen())
http.HandleFunc("/", indexHandler)
go func() {
    for {
        time.Sleep(1 * time.Second)
        data := fmt.Sprintf("Current Time: %s", time.Now().Format("2006-01-02T15:04:05.999-07:00"))
        sseBroker.SendEvent("1", "currentTime", []byte(data))
    }
}()
log.Fatal(http.ListenAndServe("127.0.0.1:5000", nil))
```

## Compression Example
```golang
ctxt := context.Background()

sseBroker := ssebroker.NewSseBroker()
http.Handle("/sse-stream", sseBroker.HandleWithContextAndGzip(ctxt))
go sseBroker.ListenWithContext(ctxt)
http.HandleFunc("/", indexHandler)
go func() {
    for {
        time.Sleep(1 * time.Second)
        data := strings.Repeat("A", 600)
        sseBroker.SendEvent("1", "currentTime", []byte(data))
    }
}()
log.Fatal(http.ListenAndServe("127.0.0.1:5000", nil))
```

## Demo-HTML
```html
<!DOCTYPE html>
<html lang="en">

<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <meta http-equiv="X-UA-Compatible" content="ie=edge">
    <title>Document</title>
</head>

<body>
    <button id=btn_close>close Stream</button>
    <label id=currentTime></label>
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
            lblCurrentTime.innerText = timeEvent.data;
        })
    </script>
</body>

</html>
```
