package session

import (
	"encoding/json"
	"testing"
)

// The viewer's record command is intercepted BEFORE the per-platform
// handleCommand. These pin that it consumes exactly its own message and nothing
// else — a greedy handler here would swallow lock/logoff/reboot/privacy, which
// all work today.

func TestRecordCmdIgnoresOtherCommands(t *testing.T) {
	for _, raw := range []string{
		`{"k":"cmd","c":"lock"}`,
		`{"k":"cmd","c":"logoff"}`,
		`{"k":"cmd","c":"reboot"}`,
		`{"k":"cmd","c":"privacy","on":true}`,
		`{"k":"cmd","c":"sas"}`,
	} {
		if handleRecordCmd([]byte(raw), nil) {
			t.Errorf("record handler swallowed %s — that command would stop working", raw)
		}
	}
}

func TestRecordCmdIgnoresNonCommands(t *testing.T) {
	// Mouse and keyboard traffic shares this path. Consuming any of it would
	// make the session unresponsive.
	for _, raw := range []string{
		`{"k":"ft","t":"offer"}`,
		`{"x":10,"y":20}`,
		`not json at all`,
		``,
	} {
		if handleRecordCmd([]byte(raw), nil) {
			t.Errorf("record handler swallowed non-command %q", raw)
		}
	}
}

func TestRecordCmdClaimsItsOwnMessage(t *testing.T) {
	// Stop with nothing recording must still be consumed, or it falls through
	// to handleCommand and is logged as an unsupported command.
	if !handleRecordCmd([]byte(`{"k":"cmd","c":"record","on":false}`), nil) {
		t.Fatal("record stop was not consumed by the record handler")
	}
	if recordingActive() {
		t.Fatal("a stop with nothing running left a recording active")
	}
}

func TestRecordCmdWithoutKnownScreenSizeDoesNotStart(t *testing.T) {
	// startRecording refuses a zero size — a WebM track declares its dimensions,
	// and one written as 0x0 does not play. Better no file than a broken one.
	setCaptureSize(0, 0)
	if !handleRecordCmd([]byte(`{"k":"cmd","c":"record","on":true}`), nil) {
		t.Fatal("record start was not consumed")
	}
	if recordingActive() {
		t.Fatal("recording started before the screen size was known")
	}
}

func TestRecordCmdShapeMatchesViewer(t *testing.T) {
	// The viewer builds this JSON; if the two ever disagree the button silently
	// does nothing, which is the failure mode this project keeps hitting.
	b, err := json.Marshal(map[string]interface{}{"k": "cmd", "c": "record", "on": true})
	if err != nil {
		t.Fatal(err)
	}
	if !handleRecordCmd(b, nil) {
		t.Fatal("the viewer's exact message shape was not recognised by the host")
	}
	setSessionBarRecording(false)
	stopRecording()
}
