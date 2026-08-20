//go:build !windows

package session

import "github.com/neev/remote-agent/agent/ipc"

// File clipboard (Ctrl+C/Ctrl+V of files) is Windows-only in TransportMode.
type clipFiles struct{}

func newClipFiles(conn *ipc.Conn) *clipFiles     { return &clipFiles{} }
func (cf *clipFiles) poll(stop <-chan struct{})  {}
func (cf *clipFiles) handle(payload []byte) bool { return false }
