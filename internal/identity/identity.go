package identity

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/foqerhk/runeverything/internal/netutil"
)

const (
	DefaultDirName = ".runeverything"
	IdentityFile   = "identity.json"
	ConfigFile     = "config.json"
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
		cfg.RelayManual = true
	}
	if cfg.PublicRelay == "" {
		cfg.PublicRelay = cfg.RelayURL
	}
	cfg.PublicRelay = netutil.ResolveClientRelay(cfg.PublicRelay, cfg.RelayURL)
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

func PlatformInfo() (osName, arch string) {
	return runtime.GOOS, runtime.GOARCH
}
