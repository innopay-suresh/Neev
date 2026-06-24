package network

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type iceServersResponse struct {
	ICEServers []ICEServer `json:"ice_servers"`
}

func (s *ICEServer) UnmarshalJSON(data []byte) error {
	type rawICEServer struct {
		URLs       json.RawMessage `json:"urls"`
		Username   string          `json:"username,omitempty"`
		Credential string          `json:"credential,omitempty"`
	}

	var raw rawICEServer
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	var urls []string
	if len(raw.URLs) > 0 && string(raw.URLs) != "null" {
		if raw.URLs[0] == '"' {
			var single string
			if err := json.Unmarshal(raw.URLs, &single); err != nil {
				return err
			}
			urls = []string{single}
		} else {
			if err := json.Unmarshal(raw.URLs, &urls); err != nil {
				return err
			}
		}
	}

	s.URLs = urls
	s.Username = raw.Username
	s.Credential = raw.Credential
	return nil
}

// ICEEndpointURL converts a WebSocket relay URL into the matching ICE server API URL.
func ICEEndpointURL(relayURL string) (string, error) {
	u, err := url.Parse(relayURL)
	if err != nil {
		return "", err
	}

	switch u.Scheme {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	case "http", "https":
		// already suitable
	default:
		if u.Scheme == "" {
			u.Scheme = "http"
		}
	}

	path := strings.TrimSuffix(u.Path, "/")
	if strings.HasSuffix(path, "/ws") {
		path = strings.TrimSuffix(path, "/ws")
	}
	u.Path = strings.TrimSuffix(path, "/") + "/api/v1/session/ice-servers"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

// FetchICEServers queries the signaling server for the current STUN/TURN configuration.
func FetchICEServers(ctx context.Context, relayURL string) ([]ICEServer, error) {
	endpoint, err := ICEEndpointURL(relayURL)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ice server request failed: %s", resp.Status)
	}

	var payload iceServersResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.ICEServers, nil
}
