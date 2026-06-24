package backend

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const maxLogEntries = 250

// LogEntry is a UI-friendly log record.
type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Raw     string `json:"raw,omitempty"`
}

type logCaptureWriter struct {
	app *App
}

func (w *logCaptureWriter) Write(p []byte) (int, error) {
	if w.app == nil {
		return len(p), nil
	}

	line := strings.TrimSpace(string(p))
	if line == "" {
		return len(p), nil
	}

	entry := LogEntry{
		Time:    time.Now().Format(time.RFC3339),
		Level:   "info",
		Message: line,
		Raw:     line,
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(line), &payload); err == nil {
		if timeValue, ok := payload["time"].(string); ok && timeValue != "" {
			entry.Time = timeValue
		}
		if levelValue, ok := payload["level"].(string); ok && levelValue != "" {
			entry.Level = levelValue
		}
		if messageValue, ok := payload["message"].(string); ok && messageValue != "" {
			entry.Message = messageValue
		} else {
			entry.Message = line
		}
	}

	w.app.appendLog(entry)
	return len(p), nil
}

// InstallLogger routes zerolog output to stderr and into the app log buffer.
func InstallLogger(app *App) {
	if app == nil {
		return
	}

	capture := &logCaptureWriter{app: app}
	log.Logger = zerolog.New(io.MultiWriter(os.Stderr, capture)).With().Timestamp().Logger()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
}

func (a *App) appendLog(entry LogEntry) {
	a.logMu.Lock()
	a.logs = append(a.logs, entry)
	if len(a.logs) > maxLogEntries {
		a.logs = append([]LogEntry(nil), a.logs[len(a.logs)-maxLogEntries:]...)
	}
	a.logMu.Unlock()

	if a.emitFn != nil {
		a.emitFn("app:log_received", entry)
	}
}

// GetLogs returns the buffered in-app logs.
func (a *App) GetLogs() []LogEntry {
	a.logMu.Lock()
	defer a.logMu.Unlock()

	return append([]LogEntry(nil), a.logs...)
}

// ClearLogs empties the buffered in-app logs.
func (a *App) ClearLogs() {
	a.logMu.Lock()
	a.logs = nil
	a.logMu.Unlock()
}
