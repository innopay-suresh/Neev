//go:build !windows

package session

// inputSink injects viewer input into the worker's session. The real
// implementation is Windows-only (SendInput); other platforms build a no-op so
// the transport/worker packages still compile and test in CI.
type inputSink interface {
	Post(raw []byte)
	Close()
}

func newInputSink() inputSink { return noopInputSink{} }

type noopInputSink struct{}

func (noopInputSink) Post(raw []byte) {}
func (noopInputSink) Close()          {}
