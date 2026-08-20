package session

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neev/remote-agent/agent/ipc"
)

// Session recordings are the largest thing this product sends, and the project
// has already shipped a transfer bug that reported success for 0 bytes. These
// pin the behaviour that matters: every byte arrives, and anything less is an
// error rather than a cheerful log line.

// collectExport runs one export over an ipc pipe and reassembles what arrived.
//
// The server side is CLOSED as soon as the exporter returns. Without that, a
// case that legitimately sends nothing (an empty or missing file) leaves the
// reader blocked in ReadMessage forever instead of finishing the test.
func collectExport(t *testing.T, run func(*fileReceiver) error) (name string, size int64, data []byte, failed string, runErr error) {
	t.Helper()
	a, b := net.Pipe()
	defer b.Close()

	server := ipc.NewConn(a)
	client := ipc.NewConn(b)
	defer client.Close()

	errCh := make(chan error, 1)
	go func() { errCh <- run(newFileReceiver(server)) }()

	// A read deadline rather than closing the pipe when the exporter returns:
	// data rides the BULK lane and is still queued in the writer goroutine at
	// that moment, so closing immediately discards the tail — the first run of
	// this test lost exactly the final partial chunk that way.
	_ = b.SetDeadline(time.Now().Add(5 * time.Second))

	for {
		kind, payload, err := client.ReadMessage()
		if err != nil {
			break // pipe closed: the exporter is done
		}
		if kind != ipc.KindFileData {
			continue
		}
		var m map[string]interface{}
		if json.Unmarshal(payload, &m) != nil {
			continue
		}
		switch m["t"] {
		case "offer":
			name, _ = m["name"].(string)
			if f, ok := m["size"].(float64); ok {
				size = int64(f)
			}
		case "data":
			chunk, _ := m["d"].(string)
			raw, derr := base64.StdEncoding.DecodeString(chunk)
			if derr != nil {
				failed = "invalid base64: " + derr.Error()
				continue
			}
			data = append(data, raw...)
		case "failed":
			failed, _ = m["err"].(string)
		}
		// Everything the offer promised has arrived.
		if size > 0 && int64(len(data)) >= size {
			break
		}
	}
	server.Close()
	a.Close()
	return name, size, data, failed, <-errCh
}

func TestStreamedExportDeliversEveryByte(t *testing.T) {
	// Deliberately not a round number of chunks: an off-by-one in the chunk
	// loop shows up as a short final block, which is exactly how a recording
	// arrives unplayable.
	payload := make([]byte, 36*1024*2+1234)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	path := filepath.Join(t.TempDir(), "session.webm")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	name, size, got, failed, runErr := collectExport(t, func(f *fileReceiver) error {
		return f.ExportFileStreaming(path)
	})
	if runErr != nil {
		t.Fatalf("export failed: %v", runErr)
	}
	if failed != "" {
		t.Fatalf("export reported failure: %s", failed)
	}
	if name != "session.webm" {
		t.Errorf("name = %q, want session.webm", name)
	}
	if size != int64(len(payload)) {
		t.Errorf("offered size %d, want %d", size, len(payload))
	}
	if len(got) != len(payload) {
		t.Fatalf("received %d bytes, want %d", len(got), len(payload))
	}
	for i := range payload {
		if got[i] != payload[i] {
			t.Fatalf("byte %d differs: got %d want %d", i, got[i], payload[i])
		}
	}
}

func TestStreamedExportRefusesEmptyFile(t *testing.T) {
	// A 0-byte recording means something went wrong upstream. Sending it as a
	// successful transfer is the exact bug this project already fixed once.
	path := filepath.Join(t.TempDir(), "empty.webm")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, _, _, gotErr := collectExport(t, func(f *fileReceiver) error {
		return f.ExportFileStreaming(path)
	})
	if gotErr == nil {
		t.Fatal("an empty file was exported as if it had succeeded")
	}
}

func TestStreamedExportRefusesMissingFile(t *testing.T) {
	_, _, _, _, gotErr := collectExport(t, func(f *fileReceiver) error {
		return f.ExportFileStreaming(filepath.Join(t.TempDir(), "nope.webm"))
	})
	if gotErr == nil {
		t.Fatal("a missing file was exported as if it had succeeded")
	}
}
