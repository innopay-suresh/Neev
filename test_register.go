package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/gorilla/websocket"
)

type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   string          `json:"error,omitempty"`
}

type RegisterPayload struct {
	AgentID        string `json:"agent_id,omitempty"`
	Hostname       string `json:"hostname"`
	OS             string `json:"os"`
	Version        string `json:"version"`
	PasswordHash   string `json:"password_hash,omitempty"`
	UnattendedHash string `json:"unattended_hash,omitempty"`
	OrgID          string `json:"org_id,omitempty"`
	DeviceGroup    string `json:"device_group,omitempty"`
	EnrollmentCode string `json:"enrollment_code,omitempty"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, "ws://localhost:8080/ws", nil)
	if err != nil {
		log.Fatalf("Dial error: %v", err)
	}
	defer conn.Close()

	hostname, _ := os.Hostname()
	payload, _ := json.Marshal(RegisterPayload{
		AgentID:        "",
		Hostname:       hostname,
		OS:             runtime.GOOS,
		Version:        "1.0.0",
		PasswordHash:   "dummyhash",
		UnattendedHash: "",
		OrgID:          "",
		DeviceGroup:    "",
		EnrollmentCode: "",
	})
	msg := Message{
		Type: "register",
		Payload: payload,
	}

	if err := conn.WriteJSON(msg); err != nil {
		log.Fatalf("Write error: %v", err)
	}

	var resp Message
	if err := conn.ReadJSON(&resp); err != nil {
		log.Fatalf("Read error: %v", err)
	}
	fmt.Printf("Response: %+v\n", resp)
}
