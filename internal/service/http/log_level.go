package http

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/grafana/alloy/internal/runtime/logging"
)

const (
	logLevelPath       = "/debug/log-level"
	logLevelResetAfter = 2 * time.Hour
)

// logLevelHandler temporarily changes Alloy's process-wide log level.
type logLevelHandler struct {
	logger     *logging.Logger
	resetAfter time.Duration

	mut        sync.Mutex
	generation uint64
	resetTimer *time.Timer
}

func newLogLevelHandler(logger *logging.Logger, resetAfter time.Duration) *logLevelHandler {
	return &logLevelHandler{
		logger:     logger,
		resetAfter: resetAfter,
	}
}

func (h *logLevelHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if requestedLevel := r.URL.Query().Get("level"); requestedLevel != "" {
		var level logging.Level
		if level.UnmarshalText([]byte(requestedLevel)) == nil {
			h.setLevel(level)
		}
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintln(w, h.logger.Level())
}

func (h *logLevelHandler) setLevel(level logging.Level) {
	h.mut.Lock()
	defer h.mut.Unlock()

	initialLevel, ok := h.logger.InitialLevel()
	if !ok {
		// The HTTP server is available while Alloy is loading its initial
		// configuration. In that small window, the current level is the only
		// startup level available.
		initialLevel = h.logger.Level()
	}

	h.generation++
	generation := h.generation
	h.logger.SetLevel(level)
	if h.resetTimer != nil {
		h.resetTimer.Stop()
	}
	h.resetTimer = time.AfterFunc(h.resetAfter, func() {
		h.mut.Lock()
		defer h.mut.Unlock()
		if h.generation != generation {
			return
		}
		h.logger.SetLevel(initialLevel)
		h.resetTimer = nil
	})
}

func (h *logLevelHandler) close() {
	h.mut.Lock()
	defer h.mut.Unlock()
	h.generation++
	if h.resetTimer != nil {
		h.resetTimer.Stop()
		h.resetTimer = nil
	}
}
