package identity

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	RelayURL     string `json:"relay_url"`
	PublicRelay  string `json:"public_relay,omitempty"` // URL advertised in QR (may differ from agent connect URL)
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
				relay = "ws://127.0.0.1:8787/ws"
			}
			return &Config{RelayURL: relay, PublicRelay: relay}, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if v := os.Getenv("RE_RELAY"); v != "" {
		cfg.RelayURL = v
	}
	if cfg.PublicRelay == "" {
		cfg.PublicRelay = cfg.RelayURL
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

func PlatformInfo() (osName, arch string) {
	return runtime.GOOS, runtime.GOARCH
}
