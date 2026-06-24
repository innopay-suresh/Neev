package core

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
)

// LocalAPIServer serves agent info on a local HTTP port so the browser UI
// can discover the agent ID and password without Wails IPC.
type LocalAPIServer struct {
	instance *AgentInstance
	listener net.Listener
	port     int
}

type agentInfoResponse struct {
	ID       string `json:"id"`
	Password string `json:"password"`
	Version  string `json:"version"`
	Platform string `json:"platform"`
}

// StartLocalAPI starts an HTTP server on port 7891 that serves agent info.
func StartLocalAPI(instance *AgentInstance, platform string) (*LocalAPIServer, error) {
	port := 7891

	mux := http.NewServeMux()

	s := &LocalAPIServer{instance: instance, port: port}

	// CORS middleware
	cors := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if r.Method == "OPTIONS" {
				w.WriteHeader(200)
				return
			}
			next(w, r)
		}
	}

	// GET /api/agent — returns agent info
	mux.HandleFunc("/api/agent", cors(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(agentInfoResponse{
			ID:       instance.GetID(),
			Password: instance.Password,
			Version:  agentVersion,
			Platform: platform,
		})
	}))

	mux.HandleFunc("/", cors(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>RemoteAgent Status</title>
  <style>
    body{font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif;margin:0;background:#f5f7fb;color:#111827}
    .wrap{max-width:760px;margin:48px auto;padding:24px}
    .card{background:#fff;border:1px solid #e5e7eb;border-radius:20px;padding:28px;box-shadow:0 12px 40px rgba(15,23,42,.08)}
    h1{margin:0 0 8px;font-size:28px}
    p{line-height:1.6;color:#4b5563}
    .grid{display:grid;grid-template-columns:160px 1fr;gap:12px 20px;margin-top:24px}
    .label{color:#6b7280;font-size:13px;text-transform:uppercase;letter-spacing:.08em}
    .value{font:600 15px/1.4 ui-monospace,SFMono-Regular,Menlo,monospace;word-break:break-word}
    .pill{display:inline-block;padding:6px 12px;border-radius:999px;background:#ecfdf5;color:#059669;font-weight:700;font-size:13px}
    .links{margin-top:24px;display:flex;gap:12px;flex-wrap:wrap}
    a{color:#2563eb;text-decoration:none;font-weight:600}
  </style>
</head>
<body>
  <div class="wrap">
    <div class="card">
      <span class="pill">Running</span>
      <h1>RemoteAgent is installed</h1>
      <p>This app keeps your device enrolled and available for remote support. It runs in the background as a service/daemon.</p>
      <div class="grid">
        <div class="label">Agent ID</div><div class="value">%s</div>
        <div class="label">Password</div><div class="value">%s</div>
        <div class="label">Version</div><div class="value">%s</div>
        <div class="label">Platform</div><div class="value">%s</div>
      </div>
      <div class="links">
        <a href="/api/agent">View JSON status</a>
        <a href="/api/health">Health check</a>
      </div>
    </div>
  </div>
</body>
</html>`,
			htmlEscape(instance.GetID()),
			htmlEscape(instance.Password),
			htmlEscape(agentVersion),
			htmlEscape(platform),
		)
	}))

	// Health check
	mux.HandleFunc("/api/health", cors(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	}))

	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return nil, fmt.Errorf("failed to bind local API port %d: %w", port, err)
	}
	s.listener = listener

	go func() {
		log.Info().Int("port", port).Msg("🌐 Local API server started")
		if err := http.Serve(listener, mux); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("local API server error")
		}
	}()

	return s, nil
}

func (s *LocalAPIServer) Stop() {
	if s.listener != nil {
		s.listener.Close()
	}
}

func (s *LocalAPIServer) Port() int {
	return s.port
}

func htmlEscape(value string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	).Replace(value)
}
