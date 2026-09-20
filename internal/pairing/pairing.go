package pairing

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/foqerhk/runeverything/internal/auth"
	"github.com/foqerhk/runeverything/internal/protocol"
	qrcode "github.com/skip2/go-qrcode"
)

const DefaultTTL = 10 * time.Minute

// Options for building a pairing payload.
type Options struct {
	NoisePub string // agent static public key (base64url)
	Version  int    // ignored; always protocol.Version
	UDP      string // host:port REUDP data plane
}

func NewPayload(relay, deviceID, name string, ttl time.Duration) (*protocol.PairingPayload, string, error) {
	return NewPayloadOpts(relay, deviceID, name, ttl, Options{})
}

func NewPayloadOpts(relay, deviceID, name string, ttl time.Duration, opt Options) (*protocol.PairingPayload, string, error) {
	token, err := auth.RandomToken(16)
	if err != nil {
		return nil, "", err
	}
	exp := time.Now().Add(ttl).Unix()
	_ = opt.Version
	p := &protocol.PairingPayload{
		V:            protocol.Version,
		Relay:        relay,
		DeviceID:     deviceID,
		PairingToken: token,
		Name:         name,
		ExpiresAt:    exp,
		NoisePub:     opt.NoisePub,
		UDP:          opt.UDP,
	}
	return p, token, nil
}

func PrintQR(p *protocol.PairingPayload) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	content := string(raw)
	qr, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "=== RunEverything Pairing ===")
	fmt.Fprintf(os.Stdout, "Device: %s (%s)\n", p.Name, p.DeviceID)
	fmt.Fprintf(os.Stdout, "Relay:  %s\n", p.Relay)
	if p.UDP != "" {
		fmt.Fprintf(os.Stdout, "UDP:    %s\n", p.UDP)
	}
	fmt.Fprintf(os.Stdout, "Proto:  v%d\n", p.V)
	if p.NoisePub != "" {
		fmt.Fprintf(os.Stdout, "Noise:  %s…\n", trimPub(p.NoisePub))
	}
	fmt.Fprintf(os.Stdout, "Expires: %s\n", time.Unix(p.ExpiresAt, 0).Format(time.RFC3339))
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, qr.ToSmallString(false))
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "JSON payload:")
	fmt.Fprintln(os.Stdout, content)
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "Deep link:")
	fmt.Fprintln(os.Stdout, p.DeepLink())
	fmt.Fprintln(os.Stdout)
	return nil
}

func trimPub(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12]
}
