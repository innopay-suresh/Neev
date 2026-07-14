package session

import "encoding/json"

// handleChat routes a viewer chat message ({"k":"chat","t":...}) to the host
// chat window. Returns true if the payload was a chat message.
func handleChat(payload []byte) bool {
	var m struct {
		K string `json:"k"`
		T string `json:"t"`
	}
	if err := json.Unmarshal(payload, &m); err != nil || m.K != "chat" {
		return false
	}
	chatShow(m.T)
	return true
}
