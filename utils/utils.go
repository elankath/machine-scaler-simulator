package utils

import (
	"fmt"
	"net/http"
	"time"
)

func SetupSSEWriter(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
}

func LogEvent(w http.ResponseWriter, msg string) {
	fmt.Fprintf(w, "[ %s ] : %s\n", time.Now().Format("2006-01-02 15:04:05"), msg)
	w.(http.Flusher).Flush()
}
