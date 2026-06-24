package network

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/grandcat/zeroconf"
	"github.com/rs/zerolog/log"
)

const (
	mdnsService = "_remote-agent._tcp"
	mdnsDomain  = "local."
)

// PeerInfo holds discovered LAN peer data.
type PeerInfo struct {
	ID       string
	Hostname string
	IP       net.IP
	Port     int
}

// MDNSServer advertises this agent on the local network.
type MDNSServer struct {
	server *zeroconf.Server
}

// NewMDNSServer starts advertising the agent on mDNS.
// agentID is the 9-digit ID; port is the agent's local listener port.
func NewMDNSServer(agentID string, port int) (*MDNSServer, error) {
	meta := []string{
		fmt.Sprintf("id=%s", agentID),
		fmt.Sprintf("version=1.0.0"),
	}
	server, err := zeroconf.Register(
		agentID, // instance name
		mdnsService,
		mdnsDomain,
		port,
		meta,
		nil, // all interfaces
	)
	if err != nil {
		return nil, fmt.Errorf("mdns register: %w", err)
	}
	log.Info().Str("id", agentID).Int("port", port).Msg("mDNS service registered")
	return &MDNSServer{server: server}, nil
}

// Shutdown stops the mDNS advertisement.
func (m *MDNSServer) Shutdown() {
	m.server.Shutdown()
}

// DiscoverLAN searches for other remote agents on the local network.
// It returns discovered peers within the given timeout.
func DiscoverLAN(ctx context.Context, timeout time.Duration) ([]PeerInfo, error) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, fmt.Errorf("mdns resolver: %w", err)
	}

	entries := make(chan *zeroconf.ServiceEntry)
	var peers []PeerInfo

	go func() {
		for entry := range entries {
			agentID := ""
			for _, txt := range entry.Text {
				var kv map[string]string
				// Parse "key=value" pairs.
				if len(txt) > 3 && txt[:3] == "id=" {
					agentID = txt[3:]
				}
				_ = kv
			}
			if agentID == "" {
				agentID = entry.Instance
			}
			ip := net.IPv4zero
			if len(entry.AddrIPv4) > 0 {
				ip = entry.AddrIPv4[0]
			} else if len(entry.AddrIPv6) > 0 {
				ip = entry.AddrIPv6[0]
			}
			peers = append(peers, PeerInfo{
				ID:       agentID,
				Hostname: entry.HostName,
				IP:       ip,
				Port:     entry.Port,
			})
			log.Info().Str("id", agentID).Str("ip", ip.String()).Int("port", entry.Port).Msg("discovered LAN peer")
		}
	}()

	ctx2, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := resolver.Browse(ctx2, mdnsService, mdnsDomain, entries); err != nil {
		return nil, err
	}

	<-ctx2.Done()
	return peers, nil
}

// DirectConnect attempts a direct TCP connection to a LAN peer.
// Returns a net.Conn ready for data exchange.
func DirectConnect(peer PeerInfo) (net.Conn, error) {
	addr := fmt.Sprintf("%s:%d", peer.IP.String(), peer.Port)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("direct connect to %s: %w", addr, err)
	}
	log.Info().Str("addr", addr).Msg("direct LAN connection established")
	return conn, nil
}

// SendJSON encodes v as JSON and writes it to conn with a newline delimiter.
func SendJSON(conn net.Conn, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = conn.Write(data)
	return err
}
