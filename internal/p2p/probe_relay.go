package p2p

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/foqerhk/runeverything/internal/registrar"
)

// ProbeRelay verifies healthz and /v1/peers (RunEverything identity), returns RTT.
func ProbeRelay(ctx context.Context, relayURL string) (time.Duration, error) {
	base, err := httpBaseFromRelay(relayURL)
	if err != nil {
		return 0, err
	}
	start := time.Now()
	client := &http.Client{Timeout: 3 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/healthz", nil)
	if err != nil {
		return 0, err
	}
	res, err := client.Do(req)
	if err != nil {
		registrar.ReportBadAsync(relayURL, "health_fail", err.Error(), "")
		return 0, err
	}
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(strings.ToLower(string(body)), "ok") {
		registrar.ReportBadAsync(relayURL, "health_fail", res.Status, "")
		return 0, errInvalidHost
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/peers", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	res, err = client.Do(req)
	if err != nil {
		registrar.ReportBadAsync(relayURL, "peers_fail", err.Error(), "")
		return 0, err
	}
	body, _ = io.ReadAll(io.LimitReader(res.Body, 1<<20))
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		registrar.ReportBadAsync(relayURL, "peers_fail", res.Status, "")
		return 0, fmt.Errorf("peers %s", res.Status)
	}
	// Must be JSON object with peers key (may be empty array).
	if !strings.Contains(string(body), "peers") {
		registrar.ReportBadAsync(relayURL, "not_relay", "missing peers field", "")
		return 0, fmt.Errorf("not a runeverything relay")
	}
	return time.Since(start), nil
}
