package registrar

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/foqerhk/runeverything/internal/identity"
	"github.com/foqerhk/runeverything/internal/netutil"
	"github.com/foqerhk/runeverything/internal/region"
)

// AutoClaimEnv enables automatic hostname + DNS via the official registrar.
func AutoClaimEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("RE_AUTO_DOMAIN")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// ClaimPublicWSS enrolls a per-node token (if needed), claims a hostname, waits for DNS.
// Call when volunteer sharing is enabled.
func ClaimPublicWSS(ctx context.Context) (wssURL, hostname string, err error) {
	base := strings.TrimSpace(os.Getenv("RE_REGISTRAR_URL"))
	if base == "" {
		base = region.GetnodeBase(ctx)
	}
	if base == "" {
		return "", "", fmt.Errorf("registrar URL empty")
	}
	ip := netutil.DetectPublicHost(ctx)
	if ip == "" || !ValidPublicIP(ip) {
		return "", "", fmt.Errorf("could not detect public IP for DNS claim")
	}
	nodeID := ""
	if id, e := identity.LoadOrCreate(); e == nil {
		nodeID = id.DeviceID
	}
	token, err := EnsureJoinToken(ctx, base, nodeID)
	if err != nil {
		return "", "", fmt.Errorf("enroll: %w", err)
	}
	cli := &Client{
		BaseURL:   base,
		JoinToken: token,
	}
	resp, err := cli.Claim(ctx, ClaimRequest{IP: ip, NodeID: nodeID, Port: 443})
	if err != nil {
		return "", "", err
	}
	wait := 5 * time.Second
	if v := os.Getenv("RE_DNS_WAIT"); v != "" {
		if d, e := time.ParseDuration(v); e == nil {
			wait = d
		}
	}
	log.Printf("registrar: claimed %s -> %s (waiting %s for DNS)", resp.Hostname, ip, wait)
	select {
	case <-ctx.Done():
		return "", "", ctx.Err()
	case <-time.After(wait):
	}
	return resp.WSSURL, resp.Hostname, nil
}

// CertStorageDir is where ACME certs are cached.
func CertStorageDir() string {
	if v := strings.TrimSpace(os.Getenv("RE_CERT_DIR")); v != "" {
		return v
	}
	home, err := identity.HomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "runeverything-certs")
	}
	return filepath.Join(home, "certs")
}
