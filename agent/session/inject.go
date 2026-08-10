package session

// inputSink injects viewer input into the worker's session.
//
// Real implementations: Windows (SendInput, inject_windows.go) and macOS
// (CGEvent, inject_darwin.go). Everywhere else the no-op keeps the
// transport/worker packages compiling and testable in CI.
//
// The interface and the no-op live HERE, with no build tag, because they used
// to sit in the !windows file. That made the no-op the automatic answer for
// every non-Windows platform — which is how macOS silently shipped without any
// input injection at all, discarding every click while every layer above
// reported success. A platform must now opt IN to the no-op by not providing a
// sink, rather than inheriting it by default.
type inputSink interface {
	Post(raw []byte)
	Close()
}

type noopInputSink struct{}

func (noopInputSink) Post(raw []byte) {}
func (noopInputSink) Close()          {}
