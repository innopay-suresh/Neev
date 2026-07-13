package session

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

// fileReceiver writes viewer→host file transfers (the {k:'ft'} protocol carried
// on the dedicated 'file' data channel) to the logged-in user's Downloads folder
// in TransportMode, where the app is UI-only and no longer hosts. The 'file'
// channel is reliable + ordered, so chunks arrive (and are appended) in order.
type fileReceiver struct {
	mu   sync.Mutex
	open map[string]*os.File
	dir  string
}

func newFileReceiver() *fileReceiver {
	return &fileReceiver{open: map[string]*os.File{}, dir: downloadsDir()}
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
		// Viewer asked the host to pick a file to send back. Needs a native file
		// picker on the (headless) host desktop — deferred.
		log.Info().Msg("worker: file 'request' (host→viewer export) not yet supported over transport")
	}
	return true
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
