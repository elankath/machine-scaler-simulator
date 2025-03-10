package webutil

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	scalesim "github.com/elankath/scaler-simulator"
)

func SetupSSEWriter(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
}

type Logger struct {
	mu sync.Mutex
}

func NewLogger() *Logger {
	return &Logger{}
}

func (l *Logger) Log(w http.ResponseWriter, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := fmt.Fprintf(w, "[ %s ] : %s\n", time.Now().Format("2006-01-02 15:04:05"), msg); err != nil {
		slog.Error("cannot write to response writer", "msg", msg, "error", err)
	}
	w.(http.Flusher).Flush()
}

func Log(w http.ResponseWriter, msg string) {
	if _, err := fmt.Fprintf(w, "[ %s ] : %s\n", time.Now().Format("2006-01-02 15:04:05"), msg); err != nil {
		slog.Error("cannot write to response writer", "msg", msg, "error", err)
	}
	w.(http.Flusher).Flush()
}

func Logf(w http.ResponseWriter, format string, a ...any) {
	msg := fmt.Sprintf(format, a)
	Log(w, msg)
}

func LogNodePodAssignments(w http.ResponseWriter, scenarioName string, nodePodAssignments []scalesim.NodePodAssignment) {
	var sb strings.Builder
	sb.WriteString("Scenario-" + scenarioName + ", NodePodAssignments Are:\n")
	for _, a := range nodePodAssignments {
		sb.WriteString(a.String())
		sb.WriteString("\n")
	}
	Log(w, sb.String())
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

func GetFloatQueryParam(r *http.Request, name string, defVal float64) (float64, error) {
	valstr := r.URL.Query().Get(name)
	if valstr == "" {
		return defVal, nil
	}
	val, err := strconv.ParseFloat(valstr, 64)
	if err != nil {
		slog.Error("cannot convert to float, using default", "value", valstr, "default", defVal)
		return defVal, err
	}
	return val, nil
}

func GetStringQueryParam(r *http.Request, name, defVal string) string {
	val := r.URL.Query().Get(name)
	if val == "" {
		return defVal
	}
	return val
}
