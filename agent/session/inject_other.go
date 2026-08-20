//go:build !windows && !darwin

package session

// No input injection on this platform: the worker still runs (capture,
// clipboard, file transfer), viewer input is simply discarded.
func newInputSink() inputSink { return noopInputSink{} }
