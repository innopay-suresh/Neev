package record

import (
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neev/remote-agent/agent/encode"
)

// A recording that cannot be opened is worse than no recording — the host
// believes they have evidence of the session and finds out otherwise later. So
// this encodes real VP8, muxes it, and hands the result to ffprobe.

func realVP8Frames(t *testing.T, n int) (frames [][]byte, keys []bool, w, h int) {
	t.Helper()
	w, h = 320, 240
	enc, err := encode.NewEncoder(w, h, 10, 300)
	if err != nil {
		t.Skipf("no VP8 encoder available: %v", err)
	}
	defer enc.Close()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < n; i++ {
		// Vary the picture so successive frames are not identical — an encoder
		// fed a static image may emit nothing at all.
		for p := 0; p < len(img.Pix); p += 4 {
			img.Pix[p] = byte((p/4 + i*7) % 255)
			img.Pix[p+1] = byte(i * 11 % 255)
			img.Pix[p+2] = byte((p / 4) % 255)
			img.Pix[p+3] = 255
		}
		ef, err := enc.Encode(img, i == 0)
		if err != nil || ef == nil || len(ef.Data) == 0 {
			continue
		}
		buf := make([]byte, len(ef.Data))
		copy(buf, ef.Data)
		frames = append(frames, buf)
		keys = append(keys, ef.IsKeyframe)
	}
	if len(frames) == 0 {
		t.Skip("encoder produced no frames")
	}
	return
}

func TestRecordingIsAPlayableWebM(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	frames, keys, w, h := realVP8Frames(t, 24)

	path := filepath.Join(t.TempDir(), "session.webm")
	r, err := New(path, w, h)
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range frames {
		if err := r.Write(f, keys[i]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.Stop(); err != nil {
		t.Fatal(err)
	}

	st, err := os.Stat(path)
	if err != nil || st.Size() == 0 {
		t.Fatalf("recording is missing or empty: %v", err)
	}

	out, err := exec.Command("ffprobe", "-v", "error",
		"-show_entries", "stream=codec_name,width,height",
		"-of", "default=noprint_wrappers=1", path).CombinedOutput()
	if err != nil {
		t.Fatalf("ffprobe rejected the recording: %v\n%s", err, out)
	}
	got := string(out)
	for _, want := range []string{"codec_name=vp8", "width=320", "height=240"} {
		if !strings.Contains(got, want) {
			t.Errorf("ffprobe output missing %q:\n%s", want, got)
		}
	}
}

func TestRecordingCutShortStillPlays(t *testing.T) {
	// The realistic failure: the machine crashes, or the session is killed, and
	// Stop never runs. Unknown-size segment and clusters exist precisely so the
	// file is still readable up to the last frame written.
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	frames, keys, w, h := realVP8Frames(t, 12)

	path := filepath.Join(t.TempDir(), "cut.webm")
	r, err := New(path, w, h)
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range frames {
		if err := r.Write(f, keys[i]); err != nil {
			t.Fatal(err)
		}
	}
	// Deliberately NOT calling Stop — simulate the process dying.
	r.f.Sync()

	out, err := exec.Command("ffprobe", "-v", "error",
		"-show_entries", "stream=codec_name", "-of", "csv=p=0", path).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "vp8") {
		t.Fatalf("an unfinished recording was not playable: %v\n%s", err, out)
	}
}

func TestEmptyFrameIsIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "e.webm")
	r, err := New(path, 320, 240)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	if err := r.Write(nil, false); err != nil {
		t.Fatal(err)
	}
	if r.Frames() != 0 {
		t.Fatal("an empty frame was counted as recorded")
	}
}
