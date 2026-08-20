package config

import (
	"fmt"
	"net"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config holds all application configuration.
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Redis   RedisConfig   `yaml:"redis"`
	DB      DBConfig      `yaml:"db"`
	TURN    TURNConfig    `yaml:"turn"`
	JWT     JWTConfig     `yaml:"jwt"`
	Auth    AuthConfig    `yaml:"auth"`
	Network NetworkConfig `yaml:"network"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	// TLSPort, when >0 with TLSCert/TLSKey set, adds a SECOND listener speaking
	// wss on this port WITHOUT disturbing the plaintext ws:// listener on Port.
	// This is what lets us roll out TLS without a flag-day: legacy ws:// clients
	// keep using Port while new builds move to wss on TLSPort; Port is retired
	// only once every client is migrated. (Phase 1 security rollout.)
	TLSPort           int      `yaml:"tls_port"`
	TLSCert           string   `yaml:"tls_cert"`
	TLSKey            string   `yaml:"tls_key"`
	TLSClientCA       string   `yaml:"tls_client_ca"`
	TLSClientCAKey    string   `yaml:"tls_client_ca_key"`
	PublicDownloadDir string   `yaml:"public_download_dir"`
	Debug             bool     `yaml:"debug"`
	AllowedOrigins    []string `yaml:"allowed_origins"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type DBConfig struct {
	DSN string `yaml:"dsn"`
}

type TURNConfig struct {
	PublicIP string `yaml:"public_ip"`
	Port     int    `yaml:"port"`
	Realm    string `yaml:"realm"`
	AuthUser string `yaml:"auth_user"`
	AuthPass string `yaml:"auth_pass"`
}

type JWTConfig struct {
	Secret      string `yaml:"secret"`
	ExpiryHours int    `yaml:"expiry_hours"`
}

type AuthConfig struct {
	Enabled               bool   `yaml:"enabled"`
	BootstrapEmail        string `yaml:"bootstrap_email"`
	BootstrapPassword     string `yaml:"bootstrap_password"`
	BootstrapPasswordHash string `yaml:"bootstrap_password_hash"`
	BootstrapRole         string `yaml:"bootstrap_role"`
}

type NetworkConfig struct {
	// STUNServers is a list of STUN server URLs.
	STUNServers []string `yaml:"stun_servers"`
	// TURNServer is the TURN server URL for relay fallback.
	TURNServer string `yaml:"turn_server"`
	// RelayURL is the WebSocket URL of this signaling server (agents use it).
	RelayURL string `yaml:"relay_url"`
	// EnrollmentCode is an optional shared enrollment secret for new agents.
	EnrollmentCode string `yaml:"enrollment_code"`
	// DefaultOrgID is applied when an agent does not send an org id.
	DefaultOrgID string `yaml:"default_org_id"`
	// DefaultDeviceGroup is applied when an agent does not send a group.
	DefaultDeviceGroup string `yaml:"default_device_group"`
}

// DefaultConfig returns production-ready defaults.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:              "0.0.0.0",
			Port:              8080,
			PublicDownloadDir: "./downloads",
			Debug:             false,
			AllowedOrigins: []string{
				"http://localhost:3000",
				"http://127.0.0.1:3000",
				"http://localhost:5173",
				"http://127.0.0.1:5173",
			},
		},
		Redis: RedisConfig{
			Addr: "localhost:6379",
			DB:   0,
		},
		DB: DBConfig{
			DSN: "postgres://remote_agent:remote_agent@localhost:5432/remote_agent?sslmode=disable",
		},
		TURN: TURNConfig{
			PublicIP: "0.0.0.0",
			Port:     3478,
			Realm:    "remote-agent.local",
			AuthUser: "agent",
			AuthPass: "changeme",
		},
		JWT: JWTConfig{
			Secret:      "CHANGE_ME_IN_PRODUCTION",
			ExpiryHours: 24,
		},
		Auth: AuthConfig{
			Enabled:       false,
			BootstrapRole: "admin",
		},
		Network: NetworkConfig{
			STUNServers: []string{
				"stun:stun.l.google.com:19302",
				"stun:stun1.l.google.com:19302",
			},
			TURNServer:         "turn:127.0.0.1:3478",
			RelayURL:           "ws://localhost:8080/ws",
			EnrollmentCode:     "",
			DefaultOrgID:       "",
			DefaultDeviceGroup: "",
		},
	}
}

// Load reads config from a YAML file, merging with defaults.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	if path == "" {
		applyEnvOverrides(cfg)
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			applyEnvOverrides(cfg)
			return cfg, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	applyEnvOverrides(cfg)
	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if secret := os.Getenv("JWT_SECRET"); secret != "" {
		cfg.JWT.Secret = secret
	}
	if expiry := os.Getenv("JWT_EXPIRY_HOURS"); expiry != "" {
		if hours, err := strconv.Atoi(expiry); err == nil {
			cfg.JWT.ExpiryHours = hours
		}
	}
	if enabled := os.Getenv("AUTH_ENABLED"); enabled != "" {
		if parsed, err := strconv.ParseBool(enabled); err == nil {
			cfg.Auth.Enabled = parsed
		}
	}
	if email := os.Getenv("AUTH_BOOTSTRAP_EMAIL"); email != "" {
		cfg.Auth.BootstrapEmail = email
	}
	if password := os.Getenv("AUTH_BOOTSTRAP_PASSWORD"); password != "" {
		cfg.Auth.BootstrapPassword = password
	}
	if hash := os.Getenv("AUTH_BOOTSTRAP_PASSWORD_HASH"); hash != "" {
		cfg.Auth.BootstrapPasswordHash = hash
	}
	if role := os.Getenv("AUTH_BOOTSTRAP_ROLE"); role != "" {
		cfg.Auth.BootstrapRole = role
	}
	if clientCA := os.Getenv("TLS_CLIENT_CA"); clientCA != "" {
		cfg.Server.TLSClientCA = clientCA
	}
	if clientCAKey := os.Getenv("TLS_CLIENT_CA_KEY"); clientCAKey != "" {
		cfg.Server.TLSClientCAKey = clientCAKey
	}
	if downloadDir := os.Getenv("PUBLIC_DOWNLOAD_DIR"); downloadDir != "" {
		cfg.Server.PublicDownloadDir = downloadDir
	}
	// Resolve TURN_PUBLIC_IP: env var overrides file config
	// If 0.0.0.0 or unset, auto-detect from network interfaces
	if turnIP := os.Getenv("TURN_PUBLIC_IP"); turnIP != "" {
		cfg.TURN.PublicIP = turnIP
	}
	if cfg.TURN.PublicIP == "" || cfg.TURN.PublicIP == "0.0.0.0" {
		detected := autoDetectIP()
		if detected != "" {
			cfg.TURN.PublicIP = detected
		}
	}
	// Always derive TURN server URL from resolved public IP + port
	if cfg.TURN.PublicIP != "" && cfg.TURN.PublicIP != "0.0.0.0" {
		cfg.Network.TURNServer = fmt.Sprintf("turn:%s:%d", cfg.TURN.PublicIP, cfg.TURN.Port)
	}
}

// autoDetectIP finds the first non-loopback, non-link-local IPv4 address
// from all network interfaces. Falls back to empty string if none is found.
func autoDetectIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			ip := ipnet.IP.To4()
			if ip == nil {
				continue
			}
			// Skip link-local (169.254.x.x)
			if ip[0] == 169 && ip[1] == 254 {
				continue
			}
			return ip.String()
		}
	}
	return ""
}
