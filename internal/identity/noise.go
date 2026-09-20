package identity

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/foqerhk/runeverything/internal/re2"
)

// Noise keys are stored alongside identity for RE2 E2E.
const NoiseKeyFile = "noise_static.json"

type noiseKeyFile struct {
	PrivateHex string `json:"private_hex"`
	PublicB64  string `json:"public_b64url"`
}

// LoadOrCreateNoiseStatic returns the device long-term Noise keypair.
func LoadOrCreateNoiseStatic() (*re2.StaticKeyPair, error) {
	dir, err := EnsureHome()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, NoiseKeyFile)
	data, err := os.ReadFile(path)
	if err == nil {
		var f noiseKeyFile
		if err := json.Unmarshal(data, &f); err != nil {
			return nil, err
		}
		priv, err := hex.DecodeString(f.PrivateHex)
		if err != nil || len(priv) != 32 {
			return nil, fmt.Errorf("invalid noise private key")
		}
		pub, err := base64.RawURLEncoding.DecodeString(f.PublicB64)
		if err != nil || len(pub) != 32 {
			return nil, fmt.Errorf("invalid noise public key")
		}
		return &re2.StaticKeyPair{Private: priv, Public: pub}, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	kp, err := re2.GenerateStaticKeyPair()
	if err != nil {
		return nil, err
	}
	f := noiseKeyFile{
		PrivateHex: hex.EncodeToString(kp.Private),
		PublicB64:  base64.RawURLEncoding.EncodeToString(kp.Public),
	}
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return nil, err
	}
	return kp, nil
}

// NoisePublicB64URL returns the agent public key for QR payloads.
func NoisePublicB64URL(kp *re2.StaticKeyPair) string {
	if kp == nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(kp.Public)
}
