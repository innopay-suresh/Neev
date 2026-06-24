package bootstrap

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Config is the bootstrap configuration loaded from file and/or environment.
type Config struct {
	RelayURL           string
	EnrollmentCode     string
	OrgID              string
	DeviceGroup        string
	UnattendedPassword string
	TURNURL            string
	TURNUser           string
	TURNPass           string
	CertFile           string
	KeyFile            string
	CAFile             string
	ConfigPath         string
}

// Load reads bootstrap values from a standard config file and environment variables.
// Environment variables always win over file values.
func Load() (Config, error) {
	cfg := Config{}
	path, err := resolveConfigPath()
	if err != nil {
		return cfg, err
	}
	if path != "" {
		if values, err := readEnvFile(path); err == nil {
			cfg.apply(values)
			cfg.ConfigPath = path
		}
	}

	cfg.apply(map[string]string{
		"RELAY_URL":           os.Getenv("RELAY_URL"),
		"ENROLLMENT_CODE":     os.Getenv("ENROLLMENT_CODE"),
		"ORG_ID":              os.Getenv("ORG_ID"),
		"DEVICE_GROUP":        os.Getenv("DEVICE_GROUP"),
		"UNATTENDED_PASSWORD": os.Getenv("UNATTENDED_PASSWORD"),
		"TURN_URL":            os.Getenv("TURN_URL"),
		"TURN_USER":           os.Getenv("TURN_USER"),
		"TURN_PASS":           os.Getenv("TURN_PASS"),
		"AGENT_CERT_FILE":     os.Getenv("AGENT_CERT_FILE"),
		"AGENT_KEY_FILE":      os.Getenv("AGENT_KEY_FILE"),
		"AGENT_CA_FILE":       os.Getenv("AGENT_CA_FILE"),
	})

	if cfg.RelayURL == "" {
		cfg.RelayURL = "ws://localhost:8080/ws"
	}
	applyDefaultCertificatePaths(&cfg)
	return cfg, nil
}

// RenderEnvFile returns a canonical agent env file for bootstrap installers.
func RenderEnvFile(cfg Config) string {
	lines := []string{
		fmt.Sprintf("RELAY_URL=%s", cfg.RelayURL),
	}
	if cfg.EnrollmentCode != "" {
		lines = append(lines, fmt.Sprintf("ENROLLMENT_CODE=%s", cfg.EnrollmentCode))
	}
	if cfg.OrgID != "" {
		lines = append(lines, fmt.Sprintf("ORG_ID=%s", cfg.OrgID))
	}
	if cfg.DeviceGroup != "" {
		lines = append(lines, fmt.Sprintf("DEVICE_GROUP=%s", cfg.DeviceGroup))
	}
	if cfg.UnattendedPassword != "" {
		lines = append(lines, fmt.Sprintf("UNATTENDED_PASSWORD=%s", cfg.UnattendedPassword))
	}
	if cfg.TURNURL != "" {
		lines = append(lines, fmt.Sprintf("TURN_URL=%s", cfg.TURNURL))
	}
	if cfg.TURNUser != "" {
		lines = append(lines, fmt.Sprintf("TURN_USER=%s", cfg.TURNUser))
	}
	if cfg.TURNPass != "" {
		lines = append(lines, fmt.Sprintf("TURN_PASS=%s", cfg.TURNPass))
	}
	if cfg.CertFile != "" {
		lines = append(lines, fmt.Sprintf("AGENT_CERT_FILE=%s", cfg.CertFile))
	}
	if cfg.KeyFile != "" {
		lines = append(lines, fmt.Sprintf("AGENT_KEY_FILE=%s", cfg.KeyFile))
	}
	if cfg.CAFile != "" {
		lines = append(lines, fmt.Sprintf("AGENT_CA_FILE=%s", cfg.CAFile))
	}
	lines = append(lines, "NO_BROWSER=1")
	return strings.Join(lines, "\n") + "\n"
}

func (c *Config) apply(values map[string]string) {
	for key, value := range values {
		value = strings.TrimSpace(value)
		value = trimQuotes(value)
		switch key {
		case "RELAY_URL":
			if value != "" {
				c.RelayURL = value
			}
		case "ENROLLMENT_CODE":
			if value != "" {
				c.EnrollmentCode = value
			}
		case "ORG_ID":
			if value != "" {
				c.OrgID = value
			}
		case "DEVICE_GROUP":
			if value != "" {
				c.DeviceGroup = value
			}
		case "UNATTENDED_PASSWORD":
			if value != "" {
				c.UnattendedPassword = value
			}
		case "TURN_URL":
			if value != "" {
				c.TURNURL = value
			}
		case "TURN_USER":
			if value != "" {
				c.TURNUser = value
			}
		case "TURN_PASS":
			if value != "" {
				c.TURNPass = value
			}
		case "AGENT_CERT_FILE":
			if value != "" {
				c.CertFile = value
			}
		case "AGENT_KEY_FILE":
			if value != "" {
				c.KeyFile = value
			}
		case "AGENT_CA_FILE":
			if value != "" {
				c.CAFile = value
			}
		}
	}
}

func resolveConfigPath() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("REMOTE_AGENT_CONFIG")); explicit != "" {
		return explicit, nil
	}

	candidates := []string{}
	switch runtime.GOOS {
	case "windows":
		if base := os.Getenv("ProgramData"); base != "" {
			candidates = append(candidates, filepath.Join(base, "RemoteAgent", "agent.env"))
		}
	case "darwin":
		candidates = append(candidates, "/Library/Application Support/RemoteAgent/agent.env")
	default:
		candidates = append(candidates,
			"/etc/remote-agent/agent.env",
			"/etc/remote-agent.env",
		)
	}

	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", nil
}

func readEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func trimQuotes(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func applyDefaultCertificatePaths(cfg *Config) {
	baseDir := defaultAgentDataDir(cfg.ConfigPath)
	if cfg.CertFile == "" {
		cfg.CertFile = filepath.Join(baseDir, "agent-cert.pem")
	}
	if cfg.KeyFile == "" {
		cfg.KeyFile = filepath.Join(baseDir, "agent-key.pem")
	}
}

func defaultAgentDataDir(configPath string) string {
	if configPath != "" {
		return filepath.Dir(configPath)
	}
	switch runtime.GOOS {
	case "windows":
		if base := os.Getenv("ProgramData"); base != "" {
			return filepath.Join(base, "RemoteAgent")
		}
	case "darwin":
		return "/Library/Application Support/RemoteAgent"
	}
	return "/etc/remote-agent"
}
