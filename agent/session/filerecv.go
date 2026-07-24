package session

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	conn *ipc.Conn // acks/export back to the transport (writes serialized)
	mu   sync.Mutex
	open map[string]*recvFile
	dir  string
	seq  atomic.Uint64
}

// recvFile tracks one in-flight incoming transfer: the open handle, its final
// unique path, the announced size, and bytes written so far — so 'end' can
// verify the file is complete (not silently truncated) before acking.
type recvFile struct {
	f       *os.File
	path    string
	size     int64
	written  int64
	lastLog  int64 // bytes at last progress log (so a stalled large file is visible)
	lastProg int64 // bytes at last {t:prog} ack sent to the viewer (flow control)
}

func newFileReceiver(conn *ipc.Conn) *fileReceiver {
	return &fileReceiver{
		conn: conn,
		open: map[string]*recvFile{},
		dir:  downloadsDir(),
	}
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
			// Tell the viewer immediately instead of leaving it to time out.
			f.sendFT(map[string]interface{}{"k": "ft", "t": "failed", "id": m.ID,
				"err": "host could not create the file"})
			return true
		}
		f.mu.Lock()
		f.open[m.ID] = &recvFile{f: file, path: path, size: m.Size}
		f.mu.Unlock()
		log.Info().Str("path", path).Int64("size", m.Size).Msg("worker: receiving file")
	case "data":
		raw, err := base64.StdEncoding.DecodeString(m.D)
		if err != nil {
			return true
		}
		f.mu.Lock()
		rf := f.open[m.ID]
		f.mu.Unlock()
		if rf != nil {
			n, werr := rf.f.Write(raw)
			rf.written += int64(n)
			if werr != nil {
				log.Warn().Err(werr).Str("id", m.ID).Msg("worker: write received chunk failed")
				f.fail(m.ID, "write error: "+werr.Error())
				return true
			}
			// Flow-control ack every ~1 MB: tell the viewer how much we've written
			// so it paces to our real drain rate instead of dumping the whole file
			// into the ~16 MB SCTP buffer (that overflow tore down the channel and
			// killed the session — large uploads died, small ones fit). Small hi-
			// lane message; ordered after the chunk it acks.
			if rf.written-rf.lastProg >= 1024*1024 {
				rf.lastProg = rf.written
				f.sendFT(map[string]interface{}{"k": "ft", "t": "prog",
					"id": m.ID, "recv": rf.written})
			}
			if rf.written-rf.lastLog >= 8*1024*1024 {
				// Progress every ~8 MB so a large-file receive is observable and a
				// stall shows exactly how far it got.
				rf.lastLog = rf.written
				log.Info().Str("id", m.ID).Int64("written", rf.written).Int64("size", rf.size).
					Msg("worker: receiving file progress")
			}
		}
	case "end":
		f.mu.Lock()
		rf := f.open[m.ID]
		delete(f.open, m.ID)
		f.mu.Unlock()
		if rf == nil {
			return true
		}
		_ = rf.f.Close()
		// Truncation guard: if what we wrote doesn't match the announced size, the
		// transfer was interrupted — report FAILED and delete the partial file
		// rather than leave corrupt data on disk that looks like a real download.
		if rf.size > 0 && rf.written != rf.size {
			_ = os.Remove(rf.path)
			log.Warn().Str("id", m.ID).Int64("want", rf.size).Int64("got", rf.written).
				Msg("worker: incomplete file — reporting failed")
			f.sendFT(map[string]interface{}{"k": "ft", "t": "failed", "id": m.ID,
				"err": fmt.Sprintf("incomplete: received %d of %d bytes", rf.written, rf.size)})
			return true
		}
		log.Info().Str("id", m.ID).Str("path", rf.path).Int64("size", rf.written).
			Msg("worker: file transfer finished")
		f.sendFT(map[string]interface{}{"k": "ft", "t": "saved", "id": m.ID, "path": rf.path})
	case "cancel":
		f.mu.Lock()
		rf := f.open[m.ID]
		delete(f.open, m.ID)
		f.mu.Unlock()
		if rf != nil {
			_ = rf.f.Close()
			_ = os.Remove(rf.path) // don't leave a partial from a cancelled transfer
			log.Info().Str("id", m.ID).Msg("worker: file transfer cancelled")
		}
	case "request":
		// Viewer asked the host for a file → pop a native picker on the host
		// desktop and stream the selection back. Runs off the reader goroutine so
		// the blocking modal dialog doesn't stall inbound messages.
		log.Info().Msg("worker: export requested by viewer — opening host file picker")
		go f.serveExport()
	}
	return true
}

// serveExport shows the host file picker and streams the chosen file to the
// viewer as {k:'ft',offer/data/end} over the transport (which relays it onto the
// viewer's 'file' channel). 36 KB raw chunks (→ ~48 KB base64) match the viewer.
func (f *fileReceiver) serveExport() {
	// The picker is a GUI dialog — bind this thread to the interactive desktop
	// first (same reason the chat window needed it), or it can fail to appear.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	bindInputDesktop()
	path, ok := showOpenFileDialog()
	if !ok {
		log.Info().Msg("worker: export picker closed/cancelled — nothing sent")
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
		f.sendFTBulk(map[string]interface{}{
			"k": "ft", "t": "data", "id": id, "seq": seq,
			"d": base64.StdEncoding.EncodeToString(data[off:end]),
		})
		seq++
	}
	f.sendFT(map[string]interface{}{"k": "ft", "t": "end", "id": id})
	log.Info().Str("name", name).Int("bytes", len(data)).Msg("worker: sent file to viewer")
}

// sendFT sends a small control message (offer/end/saved/failed) on the hi lane.
func (f *fileReceiver) sendFT(m map[string]interface{}) {
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	_ = f.conn.WriteMessage(ipc.KindFileData, b)
}

// sendFTBulk sends a bulk data chunk (host→viewer export) on the backpressured
// bulk lane so a large export never blocks input/acks on the hi lane.
func (f *fileReceiver) sendFTBulk(m map[string]interface{}) {
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	_ = f.conn.WriteBulk(ipc.KindFileData, b)
}

// fail aborts an in-flight transfer: closes + deletes the partial file and tells
// the viewer it FAILED, so it surfaces a real error instead of hanging until the
// client-side timeout — and never leaves truncated data on disk.
func (f *fileReceiver) fail(id, reason string) {
	f.mu.Lock()
	rf := f.open[id]
	delete(f.open, id)
	f.mu.Unlock()
	if rf != nil {
		_ = rf.f.Close()
		_ = os.Remove(rf.path)
	}
	f.sendFT(map[string]interface{}{"k": "ft", "t": "failed", "id": id, "err": reason})
}

// closeAll aborts any half-open transfers (worker shutdown / session swap): each
// is deleted (never leave a truncated file) and reported FAILED to the viewer.
func (f *fileReceiver) closeAll() {
	f.mu.Lock()
	ids := make([]string, 0, len(f.open))
	files := make([]*recvFile, 0, len(f.open))
	for id, rf := range f.open {
		ids = append(ids, id)
		files = append(files, rf)
		delete(f.open, id)
	}
	f.mu.Unlock()
	for i, rf := range files {
		_ = rf.f.Close()
		_ = os.Remove(rf.path)
		f.sendFT(map[string]interface{}{"k": "ft", "t": "failed", "id": ids[i],
			"err": "host session ended before the transfer completed"})
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
