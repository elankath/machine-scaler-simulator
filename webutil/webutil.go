package webutil

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

func SetupSSEWriter(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
}

func Log(w http.ResponseWriter, msg string) {
	fmt.Fprintf(w, "[ %s ] : %s\n", time.Now().Format("2006-01-02 15:04:05"), msg)
	w.(http.Flusher).Flush()
}

func HandleShootNameMissing(w http.ResponseWriter) {
	http.Error(w, "shoot param empty", http.StatusBadRequest)
}

func HandleCantGetNodes(w http.ResponseWriter, shootName string) {
	http.Error(w, fmt.Sprintf("cant get nodes for shoot %s", shootName), http.StatusBadRequest)
}

func InternalError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func GetIntPathParam(r *http.Request, name string, defVal int) int {
	valstr := r.PathValue(name)
	if valstr == "" {
		return defVal
	}
	val, err := strconv.Atoi(valstr)
	if err != nil {
		slog.Error("cannot convert to int, using default", "value", valstr, "default", defVal)
		return defVal
	}
	return val
}

func GetIntQueryParam(r *http.Request, name string, defVal int) int {
	valstr := r.URL.Query().Get(name)
	if valstr == "" {
		return defVal
	}
	val, err := strconv.Atoi(valstr)
	if err != nil {
		slog.Error("cannot convert to int, using default", "value", valstr, "default", defVal)
		return defVal
	}
	return val
}
