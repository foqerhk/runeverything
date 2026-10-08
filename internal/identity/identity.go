package identity

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/foqerhk/runeverything/internal/netutil"
)

const (
	DefaultDirName = ".runeverything"
	IdentityFile   = "identity.json"
	ConfigFile     = "config.json"
	UDPListenFile  = "udp_listen.json"
)

type Identity struct {
	DeviceID     string `json:"device_id"`
	DeviceSecret string `json:"device_secret"`
	Name         string `json:"name"`
}

type Config struct {
	RelayURL    string `json:"relay_url"`
	PublicRelay string `json:"public_relay,omitempty"` // URL advertised in QR (may differ from agent connect URL)
	// ShareRelay controls volunteer directory announce when this host runs a relay.
	// nil = default on; false = opted out. Env RE_SHARE_RELAY overrides.
	ShareRelay *bool `json:"share_relay,omitempty"`
	// RelayManual means the user pinned relay_url (do not auto-discover).
	RelayManual bool `json:"relay_manual,omitempty"`
	// IPEchoCN / IPEchoIntl override built-in public-IP echo endpoints (plain-text IP).
	// Env RE_IP_ECHO_CN / RE_IP_ECHO_INTL override these when set.
	IPEchoCN   []string `json:"ip_echo_cn,omitempty"`
	IPEchoIntl []string `json:"ip_echo_intl,omitempty"`
	// GeoURL overrides the country lookup used for region detection (ip-api JSON shape).
	// Env RE_GEO_URL overrides this when set. Force region with RE_REGION=cn|intl.
	GeoURL string `json:"geo_url,omitempty"`
}

func HomeDir() (string, error) {
	if v := os.Getenv("RE_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, DefaultDirName), nil
}

func EnsureHome() (string, error) {
	dir, err := HomeDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func RandomHex(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func LoadOrCreate() (*Identity, error) {
	dir, err := EnsureHome()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, IdentityFile)
	data, err := os.ReadFile(path)
	if err == nil {
		var id Identity
		if err := json.Unmarshal(data, &id); err != nil {
			return nil, err
		}
		if id.DeviceID == "" || id.DeviceSecret == "" {
			return nil, fmt.Errorf("invalid identity file")
		}
		return &id, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	host, _ := os.Hostname()
	if host == "" {
		host = "runeverything"
	}
	devID, err := RandomHex(16)
	if err != nil {
		return nil, err
	}
	secret, err := RandomHex(32)
	if err != nil {
		return nil, err
	}
	id := &Identity{
		DeviceID:     devID,
		DeviceSecret: secret,
		Name:         host,
	}
	if err := SaveIdentity(id); err != nil {
		return nil, err
	}
	return id, nil
}

func SaveIdentity(id *Identity) error {
	dir, err := EnsureHome()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, IdentityFile)
	raw, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func LoadConfig() (*Config, error) {
	dir, err := EnsureHome()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, ConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			relay := os.Getenv("RE_RELAY")
			if relay == "" {
				relay = "ws://127.0.0.1:8787/re2"
			}
			cfg := &Config{RelayURL: relay, PublicRelay: relay}
			cfg.PublicRelay = netutil.ResolveClientRelay(cfg.PublicRelay, cfg.RelayURL)
			return cfg, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if v := os.Getenv("RE_RELAY"); v != "" {
		cfg.RelayURL = v
		// Do not set RelayManual here: RE_RELAY already disables discovery via
		// ShouldAutoDiscover, while relay_manual is the user's persistent pin preference.
	}
	if cfg.PublicRelay == "" {
		cfg.PublicRelay = cfg.RelayURL
	}
	// Keep an explicit WAN public_relay (e.g. wss://…) — never rewrite to LAN.
	if netutil.RelayURLHostIsUnreliableOnWAN(cfg.PublicRelay) {
		cfg.PublicRelay = netutil.PairingAdvertisedRelay(cfg.PublicRelay, cfg.RelayURL)
	}
	return &cfg, nil
}

func SaveConfig(cfg *Config) error {
	dir, err := EnsureHome()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, ConfigFile)
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

// udpListenState is the sticky Agent LAN UDP port (QR lan:port).
type udpListenState struct {
	Port int `json:"port"`
}

// PreferredUDPListenPort returns RE_UDP_PORT if set, else the last sticky port
// from ~/.runeverything/udp_listen.json (0 = pick ephemeral).
func PreferredUDPListenPort() int {
	if v := strings.TrimSpace(os.Getenv("RE_UDP_PORT")); v != "" {
		var p int
		if _, err := fmt.Sscanf(v, "%d", &p); err == nil && p > 0 && p <= 65535 {
			return p
		}
	}
	dir, err := HomeDir()
	if err != nil {
		return 0
	}
	b, err := os.ReadFile(filepath.Join(dir, UDPListenFile))
	if err != nil {
		return 0
	}
	var st udpListenState
	if json.Unmarshal(b, &st) != nil || st.Port <= 0 || st.Port > 65535 {
		return 0
	}
	return st.Port
}

// SaveUDPListenPort remembers the bound LAN UDP port for the next Agent start.
func SaveUDPListenPort(port int) error {
	if port <= 0 || port > 65535 {
		return nil
	}
	dir, err := EnsureHome()
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(udpListenState{Port: port}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, UDPListenFile), raw, 0o600)
}

func PlatformInfo() (osName, arch string) {
	return runtime.GOOS, runtime.GOARCH
}
