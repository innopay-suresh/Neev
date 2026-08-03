//go:build !windows

package session

// The host session bar is a native Windows window today. Off Windows the host
// ends a session from the Flutter UI instead, so these are no-ops rather than
// stubs that pretend to show something.
func showHostSessionBar(onHangUp func()) {}
func hideHostSessionBar()                {}
