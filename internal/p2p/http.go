package p2p

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

func httpBaseFromRelay(relayURL string) (string, error) {
	u, err := url.Parse(relayURL)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "wss":
		u.Scheme = "https"
	case "ws":
		u.Scheme = "http"
	default:
		return "", errInvalidScheme
	}
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	return stringsTrimRightSlash(u.String()), nil
}

func stringsTrimRightSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

// FetchPeers GETs /v1/peers from a relay.
func FetchPeers(ctx context.Context, relayURL string) (PeersResponse, error) {
	var zero PeersResponse
	base, err := httpBaseFromRelay(relayURL)
	if err != nil {
		return zero, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/peers", nil)
	if err != nil {
		return zero, err
	}
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 4 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return zero, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return zero, err
	}
	if res.StatusCode != http.StatusOK {
		return zero, fmt.Errorf("peers %s: %s", relayURL, res.Status)
	}
	var resp PeersResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return zero, err
	}
	return resp, nil
}

// PushPeers POSTs addr gossip to a relay.
func PushPeers(ctx context.Context, relayURL string, msg AnnouncePeers) error {
	base, err := httpBaseFromRelay(relayURL)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/peers", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 4 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("push %s: %s", relayURL, res.Status)
	}
	return nil
}

// ProbeHealth measures RTT via GET /healthz.
func ProbeHealth(ctx context.Context, relayURL string) (time.Duration, error) {
	base, err := httpBaseFromRelay(relayURL)
	if err != nil {
		return 0, err
	}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/healthz", nil)
	if err != nil {
		return 0, err
	}
	client := &http.Client{Timeout: 3 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 0, errInvalidHost
	}
	return time.Since(start), nil
}
