package session

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"

	"github.com/neev/remote-agent/agent/ipc"
)

// fileReceiver carries file transfers over the transport in TransportMode (the
// app is UI-only and no longer hosts). Viewer→host (import) chunks are written
// to the user's Downloads folder; host→viewer (export) pops a native picker on
// the user's desktop and streams the chosen file back. The 'file' channel is
// reliable + ordered, so chunks arrive/append in order.
type fileReceiver struct {
	conn net.Conn // to send export data back to the transport
	mu   sync.Mutex
	open map[string]*os.File
	dir  string
	seq  atomic.Uint64
}

func newFileReceiver(conn net.Conn) *fileReceiver {
	return &fileReceiver{conn: conn, open: map[string]*os.File{}, dir: downloadsDir()}
}

// downloadsDir returns the user's Downloads folder (the worker runs as that
// user), falling back to the home dir then the temp dir.
func downloadsDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return os.TempDir()
	}
	d := filepath.Join(home, "Downloads")
	if fi, err := os.Stat(d); err == nil && fi.IsDir() {
		return d
	}
	return home
}

// handle processes one {k:'ft'} message. Returns true if the payload was a
// file-transfer message (so the caller doesn't treat it as anything else).
// The host→viewer "request" (export) needs a picker on the headless host and is
// not carried yet — it's consumed here so it doesn't error downstream.
func (f *fileReceiver) handle(payload []byte) bool {
	var m struct {
		K    string `json:"k"`
		T    string `json:"t"`
		ID   string `json:"id"`
		Name string `json:"name"`
		D    string `json:"d"`
		Seq  int    `json:"seq"`
		Size int64  `json:"size"`
	}
	if err := json.Unmarshal(payload, &m); err != nil || m.K != "ft" {
		return false
	}
	switch m.T {
	case "offer":
		path := f.uniquePath(m.Name)
		file, err := os.Create(path)
		if err != nil {
			log.Warn().Err(err).Str("name", m.Name).Msg("worker: create received file failed")
			return true
		}
		f.mu.Lock()
		f.open[m.ID] = file
		f.mu.Unlock()
		log.Info().Str("path", path).Int64("size", m.Size).Msg("worker: receiving file")
	case "data":
		raw, err := base64.StdEncoding.DecodeString(m.D)
		if err != nil {
			return true
		}
		f.mu.Lock()
		file := f.open[m.ID]
		f.mu.Unlock()
		if file != nil {
			_, _ = file.Write(raw)
		}
	case "end", "cancel":
		f.mu.Lock()
		file := f.open[m.ID]
		delete(f.open, m.ID)
		f.mu.Unlock()
		if file != nil {
			_ = file.Close()
			log.Info().Str("id", m.ID).Str("t", m.T).Msg("worker: file transfer finished")
		}
	case "request":
		// Viewer asked the host for a file → pop a native picker on the host
		// desktop and stream the selection back. Runs off the reader goroutine so
		// the blocking modal dialog doesn't stall inbound messages.
		go f.serveExport()
	}
	return true
}

// serveExport shows the host file picker and streams the chosen file to the
// viewer as {k:'ft',offer/data/end} over the transport (which relays it onto the
// viewer's 'file' channel). 36 KB raw chunks (→ ~48 KB base64) match the viewer.
func (f *fileReceiver) serveExport() {
	path, ok := showOpenFileDialog()
	if !ok {
		return // cancelled
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Warn().Err(err).Str("path", path).Msg("worker: read export file failed")
		return
	}
	id := fmt.Sprintf("hx-%d", f.seq.Add(1))
	name := filepath.Base(path)
	f.sendFT(map[string]interface{}{"k": "ft", "t": "offer", "id": id, "name": name, "size": len(data)})
	const chunk = 36 * 1024
	seq := 0
	for off := 0; off < len(data); off += chunk {
		end := off + chunk
		if end > len(data) {
			end = len(data)
		}
		f.sendFT(map[string]interface{}{
			"k": "ft", "t": "data", "id": id, "seq": seq,
			"d": base64.StdEncoding.EncodeToString(data[off:end]),
		})
		seq++
	}
	f.sendFT(map[string]interface{}{"k": "ft", "t": "end", "id": id})
	log.Info().Str("name", name).Int("bytes", len(data)).Msg("worker: sent file to viewer")
}

func (f *fileReceiver) sendFT(m map[string]interface{}) {
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	_ = ipc.WriteMessage(f.conn, ipc.KindFileData, b)
}

// closeAll releases any half-open transfers (worker shutdown / session swap).
func (f *fileReceiver) closeAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, file := range f.open {
		_ = file.Close()
		delete(f.open, id)
	}
}

func (f *fileReceiver) uniquePath(name string) string {
	name = sanitizeName(name)
	if name == "" {
		name = "file"
	}
	p := filepath.Join(f.dir, name)
	if _, err := os.Stat(p); err != nil {
		return p
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; i < 1000; i++ {
		q := filepath.Join(f.dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
		if _, err := os.Stat(q); err != nil {
			return q
		}
	}
	return p
}

// sanitizeName strips any path and illegal filename characters so a viewer can't
// write outside the Downloads folder.
func sanitizeName(n string) string {
	n = filepath.Base(n)
	n = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', 0:
			return '_'
		}
		return r
	}, n)
	return strings.TrimSpace(n)
}
