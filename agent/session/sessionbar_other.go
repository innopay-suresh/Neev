//go:build !windows && !darwin

package session

// The host session bar is a native Windows window, and macOS has its own
// menu-bar helper. Everywhere else the host ends a session from the Flutter UI
// instead, so these are no-ops rather than stubs that pretend to show something.
func showHostSessionBar(onHangUp func()) {}
func hideHostSessionBar()                {}

// showHostSessionBarWithVoice is likewise a no-op off Windows. A host there has
// no native bar, so there is no host-owned microphone control either — and
// without one the host's mic stays shut, which is the safe direction to fail.
func showHostSessionBarWithVoice(onHangUp func(), onTalk func(string, bool)) {}

// setSessionBarRecording is a no-op where there is no native bar to update.
func setSessionBarRecording(on bool) {}
