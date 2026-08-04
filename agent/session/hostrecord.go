package session

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/neev/remote-agent/agent/record"
)

// Session recording.
//
// Recorded in the WORKER, from the frames it has already encoded — a mux, not a
// re-encode, so it costs almost nothing on top of the live stream.
//
// Started only by the HOST, from the session bar, and never by the viewer. A
// remote party who could silently start recording the screen of the machine
// they connected to would make this a surveillance tool. The host records their
// own session, for their own evidence of what was done on their machine.

var (
	recMu  sync.Mutex
	recCur *record.Recorder

	// Current capture size, published by the capture loop. The record toggle
	// arrives on the IPC reader goroutine, which cannot see the capturer, and a
	// recording opened with the wrong dimensions plays back stretched.
	recW atomic.Int32
	recH atomic.Int32
)

// setCaptureSize records the live screen dimensions for any recording started
// from another goroutine. Called on start and on every resolution change.
func setCaptureSize(w, h int) {
	recW.Store(int32(w))
	recH.Store(int32(h))
}

// capturerBounds returns the last known capture size.
func capturerBounds() (int, int) {
	return int(recW.Load()), int(recH.Load())
}

// recordingsDir is somewhere the host can actually find the file. Documents
// rather than an application-support folder: a recording nobody can locate is
// not evidence of anything.
func recordingsDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		d := filepath.Join(home, "Documents", "Neev Recordings")
		if err := os.MkdirAll(d, 0o755); err == nil {
			return d
		}
	}
	d := filepath.Join(userDataDir(), "recordings")
	_ = os.MkdirAll(d, 0o755)
	return d
}

// startRecording begins capturing this session to a file.
func startRecording(width, height int) (string, bool) {
	recMu.Lock()
	defer recMu.Unlock()
	if recCur != nil {
		return recCur.Path(), true
	}
	if width <= 0 || height <= 0 {
		log.Warn().Msg("worker: cannot record before the screen size is known")
		return "", false
	}
	name := fmt.Sprintf("session-%s.webm", time.Now().Format("2006-01-02-150405"))
	path := filepath.Join(recordingsDir(), name)
	r, err := record.New(path, width, height)
	if err != nil {
		log.Warn().Err(err).Msg("worker: could not start recording")
		return "", false
	}
	recCur = r
	log.Info().Str("path", path).Msg("worker: recording started")
	return path, true
}

// writeRecordingFrame appends a frame if a recording is running.
//
// Failures stop the recording rather than being retried forever: a full disk
// would otherwise log on every frame and quietly produce a corrupt file that
// looks like a successful capture.
func writeRecordingFrame(vp8 []byte, keyframe bool) {
	recMu.Lock()
	r := recCur
	recMu.Unlock()
	if r == nil {
		return
	}
	if err := r.Write(vp8, keyframe); err != nil {
		log.Warn().Err(err).Msg("worker: recording failed — stopping it")
		stopRecording()
	}
}

// stopRecording finishes the current recording and returns its path.
func stopRecording() string {
	recMu.Lock()
	r := recCur
	recCur = nil
	recMu.Unlock()
	if r == nil {
		return ""
	}
	path, err := r.Stop()
	if err != nil {
		log.Warn().Err(err).Msg("worker: recording did not close cleanly")
	}
	log.Info().Str("path", path).Int("frames", r.Frames()).
		Msg("worker: recording stopped")
	return path
}

// deliverRecording hands a finished recording to the connected viewer.
//
// Safe to do without asking: the viewer WATCHED this session live, so the
// recording contains nothing they have not already seen. What it adds is a
// copy for the person who usually wants one — the technician — without which
// the file sits on the host's disk and the feature helps nobody remotely.
//
// The host keeps their copy either way; this sends, it does not move.
func deliverRecording(f *fileReceiver, path string) {
	if f == nil || path == "" {
		return
	}
	if err := f.ExportFileStreaming(path); err != nil {
		// The host's copy is already on disk, so a failed hand-off costs the
		// recording nothing — say so plainly and leave it where it is.
		log.Warn().Err(err).Str("path", path).
			Msg("worker: could not send the recording to the viewer — it is still saved on this machine")
		return
	}
	log.Info().Str("path", path).Msg("worker: recording sent to viewer")
}

// recordingActive reports whether a recording is running.
func recordingActive() bool {
	recMu.Lock()
	defer recMu.Unlock()
	return recCur != nil
}
