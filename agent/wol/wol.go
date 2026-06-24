package wol

import (
	"fmt"
	"net"
	"os"
	"runtime"

	log "github.com/rs/zerolog/log"
)

// WoLPort is the standard WoL UDP port.
const WoLPort = 9

// GetPrimaryMAC returns the MAC address of the primary network interface.
// It logs the MAC address on startup for debugging purposes.
func GetPrimaryMAC() (net.HardwareAddr, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	// Find the first suitable interface
	for _, iface := range interfaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		// Skip interfaces without MAC address
		if len(iface.HardwareAddr) != 6 {
			continue
		}
		// On macOS, skip interfaces like appletv, bridge, etc. that are not the primary ethernet/wifi
		if runtime.GOOS == "darwin" {
			if iface.Name == "awdl0" || iface.Name == "llw0" || iface.Name == "bridge0" || iface.Name == "utun0" || iface.Name == "utun1" {
				continue
			}
		}
		// On Windows, skip virtual/adapter interfaces
		if runtime.GOOS == "windows" {
			if len(iface.Name) > 3 && (iface.Name[:3] == "vEthernet" || iface.Name[:3] == "VPN" || iface.Name[:3] == "Loop") {
				continue
			}
		}

		log.Info().Str("interface", iface.Name).Str("mac", iface.HardwareAddr.String()).Msg("WoL: primary network interface MAC address")
		return iface.HardwareAddr, nil
	}

	return nil, fmt.Errorf("no suitable network interface found for WoL")
}

// BuildMagicPacket creates a WoL magic packet for the given MAC address.
// Format: 6 bytes of 0xFF followed by 16 repetitions of the 6-byte MAC address (total 102 bytes).
func BuildMagicPacket(mac net.HardwareAddr) []byte {
	packet := make([]byte, 6+16*6)
	// First 6 bytes are 0xFF
	for i := 0; i < 6; i++ {
		packet[i] = 0xFF
	}
	// Followed by 16 repetitions of the MAC address
	for i := 0; i < 16; i++ {
		offset := 6 + i*6
		copy(packet[offset:], mac)
	}
	return packet
}

// SendMagicPacket sends a WoL magic packet to the broadcast address (255.255.255.255:9)
// for the given MAC address.
func SendMagicPacket(mac net.HardwareAddr) error {
	packet := BuildMagicPacket(mac)

	// Create UDP connection to broadcast address
	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{
		IP:   net.IPv4(255, 255, 255, 255),
		Port: WoLPort,
	})
	if err != nil {
		return fmt.Errorf("failed to create UDP connection for WoL: %w", err)
	}
	defer conn.Close()

	// Allow broadcasting
	if err := conn.SetWriteBuffer(1024); err != nil {
		log.Warn().Err(err).Msg("WoL: failed to set write buffer size")
	}

	n, err := conn.Write(packet)
	if err != nil {
		return fmt.Errorf("failed to send WoL magic packet: %w", err)
	}

	if n != len(packet) {
		return fmt.Errorf("incomplete WoL magic packet sent: %d/%d bytes", n, len(packet))
	}

	log.Info().Str("mac", mac.String()).Int("bytes", n).Msg("WoL: magic packet sent successfully")
	return nil
}

// SendMagicPacketToMACString sends a WoL magic packet to the MAC address string.
// The MAC string can be in format "AA:BB:CC:DD:EE:FF" or "AABBCCDDEEFF".
func SendMagicPacketToMACString(macStr string) error {
	mac, err := net.ParseMAC(macStr)
	if err != nil {
		return fmt.Errorf("invalid MAC address %q: %w", macStr, err)
	}
	return SendMagicPacket(mac)
}

// LogMACOnStartup logs the primary MAC address for debugging purposes.
func LogMACOnStartup() {
	mac, err := GetPrimaryMAC()
	if err != nil {
		log.Warn().Err(err).Msg("WoL: could not determine primary network interface MAC address")
		return
	}
	log.Info().Str("mac", mac.String()).Msg("WoL: agent MAC address (for Wake-on-LAN)")
}

// FormatMAC returns a formatted MAC address string (XX:XX:XX:XX:XX:XX).
func FormatMAC(mac net.HardwareAddr) string {
	return mac.String()
}

// ParseMAC parses a MAC address string and returns a net.HardwareAddr.
// Supports formats: "XX:XX:XX:XX:XX:XX", "XX-XX-XX-XX-XX-XX", "XXXXXXXXXXXX".
func ParseMAC(s string) (net.HardwareAddr, error) {
	return net.ParseMAC(s)
}

// SaveMACToFile saves the MAC address to a file for reference.
func SaveMACToFile(mac net.HardwareAddr, filepath string) error {
	return os.WriteFile(filepath, []byte(mac.String()), 0644)
}