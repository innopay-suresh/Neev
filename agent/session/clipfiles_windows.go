//go:build windows

package session

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/neev/remote-agent/agent/ipc"
)

// clipFiles is the HOST end of the file-clipboard protocol in TransportMode,
// reusing the existing clipf* messages (the viewer already implements the other
// end incl. delayed-render) and the neev_helper "clipagent" (127.0.0.1:47922)
// for the host's CF_HDROP read/write. Ctrl+C a file on the host → viewer pastes;
// Ctrl+C on the viewer → host pastes.
//
//   As SOURCE (host copied):     poll clipagent 'R' → clipfann → (on clipfreq) read bytes → clipfdat
//   As DESTINATION (viewer copied): clipfann → clipfreq (eager) → assemble clipfdat → temp file → clipagent 'W'
type clipFiles struct {
	conn net.Conn // to the transport (clipf* ride ipc.KindFileData → viewer file channel)

	mu       sync.Mutex
	lastRead string                 // last host CF_HDROP paths seen (echo/change guard)
	outFiles map[string][]string    // token → host paths we announced (serve on clipfreq)
	seq      int                    // token counter
	inAsm    map[string]*clipInASM  // token → destination assembly (viewer→host)
}

type clipInASM struct {
	names        []string
	parts        map[int][]string // index → base64 chunk parts in seq order
	got          map[int]int      // index → next expected seq
	total        map[int]int      // index → total chunks
	done         map[int]bool
	pending      int      // files still to complete
	pathsWritten []string // temp files written, to stage on the host clipboard
}

func newClipFiles(conn net.Conn) *clipFiles {
	return &clipFiles{conn: conn, outFiles: map[string][]string{}, inAsm: map[string]*clipInASM{}}
}

func (cf *clipFiles) send(m map[string]interface{}) {
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	_ = ipc.WriteMessage(cf.conn, ipc.KindFileData, b)
}

// poll watches the HOST clipboard (via the clipagent) for a file copy and
// announces it to the viewer. Bytes move only when the viewer pastes (clipfreq).
func (cf *clipFiles) poll(stop <-chan struct{}) {
	t := time.NewTicker(700 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}
		paths, ok := clipAgentReadFiles()
		if !ok {
			continue
		}
		joined := strings.Join(paths, "\n")
		cf.mu.Lock()
		changed := joined != cf.lastRead
		cf.lastRead = joined
		cf.mu.Unlock()
		if !changed || len(paths) == 0 {
			continue
		}
		cf.announce(paths)
	}
}

func (cf *clipFiles) announce(paths []string) {
	files := make([]map[string]interface{}, 0, len(paths))
	kept := make([]string, 0, len(paths))
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil || fi.IsDir() || fi.Size() > clipFileMaxBytes {
			continue // folders/huge files not mirrored
		}
		files = append(files, map[string]interface{}{"name": filepath.Base(p), "size": fi.Size()})
		kept = append(kept, p)
	}
	if len(files) == 0 {
		return
	}
	cf.mu.Lock()
	cf.seq++
	token := "h" + itoa(cf.seq)
	cf.outFiles[token] = kept
	if len(cf.outFiles) > 8 {
		for k := range cf.outFiles { // drop an arbitrary oldest-ish entry
			delete(cf.outFiles, k)
			break
		}
	}
	cf.mu.Unlock()
	log.Info().Str("token", token).Int("files", len(files)).Msg("worker: announcing host clipboard files")
	cf.send(map[string]interface{}{"k": "clipfann", "token": token, "files": files})
}

const clipFileMaxBytes = 64 * 1024 * 1024 // 64 MB cap, matches the viewer

// handle routes an inbound clipf* message (from the viewer via the file channel).
// Returns true if it was a clipf* message.
func (cf *clipFiles) handle(payload []byte) bool {
	var m struct {
		K     string `json:"k"`
		Token string `json:"token"`
		Index int    `json:"index"`
		OK    bool   `json:"ok"`
		Seq   int    `json:"seq"`
		Total int    `json:"total"`
		D     string `json:"d"`
		Files []struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
		} `json:"files"`
	}
	if err := json.Unmarshal(payload, &m); err != nil {
		return false
	}
	switch m.K {
	case "clipfann":
		// Viewer copied file(s) → eagerly pull each and stage on the host clipboard.
		names := make([]string, len(m.Files))
		asm := &clipInASM{names: names, parts: map[int][]string{}, got: map[int]int{},
			total: map[int]int{}, done: map[int]bool{}, pending: len(m.Files)}
		for i, f := range m.Files {
			names[i] = f.Name
		}
		cf.mu.Lock()
		cf.inAsm[m.Token] = asm
		cf.mu.Unlock()
		log.Info().Str("token", m.Token).Int("files", len(m.Files)).Msg("worker: viewer announced clipboard files; fetching")
		for i := range m.Files {
			cf.send(map[string]interface{}{"k": "clipfreq", "token": m.Token, "index": i})
		}
		return true
	case "clipfreq":
		// Viewer is pasting one of the host's announced files → send its bytes.
		cf.mu.Lock()
		paths := cf.outFiles[m.Token]
		cf.mu.Unlock()
		if paths == nil || m.Index < 0 || m.Index >= len(paths) {
			cf.send(map[string]interface{}{"k": "clipfdat", "token": m.Token, "index": m.Index, "ok": false, "seq": 0, "total": 1})
			return true
		}
		cf.serveBytes(m.Token, m.Index, paths[m.Index])
		return true
	case "clipfdat":
		// Bytes for a file we (host destination) requested from the viewer.
		cf.recvBytes(m.Token, m.Index, m.OK, m.Seq, m.Total, m.D)
		return true
	}
	return false
}

// serveBytes reads a host file and streams it to the viewer in 48KB base64 chunks.
func (cf *clipFiles) serveBytes(token string, index int, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		cf.send(map[string]interface{}{"k": "clipfdat", "token": token, "index": index, "ok": false, "seq": 0, "total": 1})
		return
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	const chunk = 48 * 1024
	total := (len(b64) + chunk - 1) / chunk
	if total < 1 {
		total = 1
	}
	for i := 0; i < total; i++ {
		start := i * chunk
		end := start + chunk
		if end > len(b64) {
			end = len(b64)
		}
		cf.send(map[string]interface{}{
			"k": "clipfdat", "token": token, "index": index, "ok": true,
			"seq": i, "total": total, "d": b64[start:end],
		})
	}
}

// recvBytes assembles a viewer file (host destination) and, when all announced
// files are complete, writes them to a temp folder and puts them on the host
// clipboard as CF_HDROP via the clipagent.
func (cf *clipFiles) recvBytes(token string, index int, okFlag bool, seq, total int, d string) {
	cf.mu.Lock()
	asm := cf.inAsm[token]
	cf.mu.Unlock()
	if asm == nil {
		return
	}
	if !okFlag {
		cf.finishFile(asm, token, index, nil)
		return
	}
	cf.mu.Lock()
	if seq == 0 {
		asm.parts[index] = make([]string, total)
		asm.total[index] = total
		asm.got[index] = 0
	}
	if asm.total[index] == total && seq == asm.got[index] && seq < len(asm.parts[index]) {
		asm.parts[index][seq] = d
		asm.got[index]++
	}
	complete := asm.got[index] == asm.total[index] && asm.total[index] > 0
	var b64 string
	if complete {
		b64 = strings.Join(asm.parts[index], "")
		asm.parts[index] = nil
	}
	cf.mu.Unlock()
	if !complete {
		return
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		cf.finishFile(asm, token, index, nil)
		return
	}
	cf.finishFile(asm, token, index, raw)
}

func (cf *clipFiles) finishFile(asm *clipInASM, token string, index int, data []byte) {
	cf.mu.Lock()
	if asm.done[index] {
		cf.mu.Unlock()
		return
	}
	asm.done[index] = true
	asm.pending--
	name := "file"
	if index >= 0 && index < len(asm.names) && asm.names[index] != "" {
		name = sanitizeName(asm.names[index])
	}
	allDone := asm.pending <= 0
	cf.mu.Unlock()

	var written string
	if data != nil {
		dir := filepath.Join(os.TempDir(), "NeevClip")
		_ = os.MkdirAll(dir, 0o755)
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0o644); err == nil {
			written = p
		}
	}
	cf.mu.Lock()
	if written != "" {
		asm.pathsWritten = append(asm.pathsWritten, written)
	}
	paths := asm.pathsWritten
	cf.mu.Unlock()

	if allDone {
		if len(paths) > 0 {
			if clipAgentWriteFiles(paths) {
				log.Info().Str("token", token).Int("files", len(paths)).Msg("worker: staged viewer files on host clipboard")
			} else {
				log.Warn().Msg("worker: clipagent write files failed")
			}
		}
		cf.mu.Lock()
		delete(cf.inAsm, token)
		cf.mu.Unlock()
	}
}

// ---- clipagent client (neev_helper 'clipagent' on 127.0.0.1:47922) ----------
// Framing: [uint32 BE len][1 byte type][payload]. 'R'→'F'(\n-joined paths); 'W'(paths)→'K'/'E'.

func clipAgentReadFiles() ([]string, bool) {
	c, err := net.DialTimeout("tcp", "127.0.0.1:47922", 2*time.Second)
	if err != nil {
		return nil, false
	}
	defer c.Close()
	if !clipAgentSend(c, 'R', nil) {
		return nil, false
	}
	typ, payload, ok := clipAgentRecv(c)
	if !ok || typ != 'F' {
		return nil, false
	}
	if len(payload) == 0 {
		return nil, true
	}
	var out []string
	for _, p := range strings.Split(string(payload), "\n") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out, true
}

func clipAgentWriteFiles(paths []string) bool {
	c, err := net.DialTimeout("tcp", "127.0.0.1:47922", 2*time.Second)
	if err != nil {
		return false
	}
	defer c.Close()
	if !clipAgentSend(c, 'W', []byte(strings.Join(paths, "\n"))) {
		return false
	}
	typ, _, ok := clipAgentRecv(c)
	return ok && typ == 'K'
}

func clipAgentSend(c net.Conn, typ byte, payload []byte) bool {
	buf := make([]byte, 5+len(payload))
	binary.BigEndian.PutUint32(buf[0:4], uint32(1+len(payload)))
	buf[4] = typ
	copy(buf[5:], payload)
	_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := c.Write(buf)
	return err == nil
}

func clipAgentRecv(c net.Conn) (byte, []byte, bool) {
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	var lb [4]byte
	if _, err := io.ReadFull(c, lb[:]); err != nil {
		return 0, nil, false
	}
	l := binary.BigEndian.Uint32(lb[:])
	if l < 1 || l > 64*1024*1024 {
		return 0, nil, false
	}
	buf := make([]byte, l)
	if _, err := io.ReadFull(c, buf); err != nil {
		return 0, nil, false
	}
	return buf[0], buf[1:], true
}
