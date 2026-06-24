package network

import (
	"encoding/json"
	"testing"
)

func TestICEServerUnmarshalAcceptsStringAndArrayURLs(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		var server ICEServer
		if err := json.Unmarshal([]byte(`{"urls":"turn:turn.example.com:3478","username":"agent","credential":"secret"}`), &server); err != nil {
			t.Fatalf("unmarshal string urls: %v", err)
		}
		if len(server.URLs) != 1 || server.URLs[0] != "turn:turn.example.com:3478" {
			t.Fatalf("unexpected urls: %#v", server.URLs)
		}
		if server.Username != "agent" || server.Credential != "secret" {
			t.Fatalf("unexpected auth fields: %#v", server)
		}
	})

	t.Run("array", func(t *testing.T) {
		var server ICEServer
		if err := json.Unmarshal([]byte(`{"urls":["stun:stun.l.google.com:19302","stun:stun1.l.google.com:19302"]}`), &server); err != nil {
			t.Fatalf("unmarshal array urls: %v", err)
		}
		if len(server.URLs) != 2 || server.URLs[0] != "stun:stun.l.google.com:19302" || server.URLs[1] != "stun:stun1.l.google.com:19302" {
			t.Fatalf("unexpected urls: %#v", server.URLs)
		}
	})
}
