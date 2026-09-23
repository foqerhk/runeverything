package directory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// DirectoryBase returns the discovery base used for GET list (env RE_DIRECTORY).
// Empty when unset — discovery defaults to P2P seeds, not a static relays.json mirror.
func DirectoryBase() string {
	if v := strings.TrimSpace(os.Getenv("RE_DIRECTORY")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return ""
}

// AnnounceEndpoint returns where volunteers POST heartbeats (env RE_DIRECTORY_ANNOUNCE).
// Falls back to DirectoryBase + "/v1/relays/announce" when base looks like a live API
// (not a raw .json file).
func AnnounceEndpoint() string {
	if v := strings.TrimSpace(os.Getenv("RE_DIRECTORY_ANNOUNCE")); v != "" {
		return v
	}
	base := DirectoryBase()
	if strings.HasSuffix(base, ".json") {
		return DefaultAnnounceURL
	}
	if strings.HasSuffix(base, "/v1/relays") {
		return base + "/announce"
	}
	return base + "/v1/relays/announce"
}

// FetchRelays loads volunteer relay entries from a self-hosted directory (RE_DIRECTORY).
func FetchRelays(ctx context.Context) ([]Entry, error) {
	base := DirectoryBase()
	if base == "" {
		return nil, fmt.Errorf("RE_DIRECTORY not set (P2P seeds are the default discovery path)")
	}
	listURL := base
	if !strings.HasSuffix(base, ".json") && !strings.Contains(base, "/v1/relays") {
		listURL = base + "/v1/relays"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("directory %s: %s", listURL, res.Status)
	}
	return parseListBody(body)
}

func parseListBody(body []byte) ([]Entry, error) {
	body = bytes.TrimSpace(body)
	// Shape A: { "relays": [ ... ] }
	var wrapped ListResponse
	if err := json.Unmarshal(body, &wrapped); err == nil && wrapped.Relays != nil {
		return wrapped.Relays, nil
	}
	// Shape B: bare array
	var arr []Entry
	if err := json.Unmarshal(body, &arr); err == nil {
		return arr, nil
	}
	return nil, fmt.Errorf("unrecognized directory payload")
}

// PostAnnounce sends a volunteer heartbeat. No-op if announce URL is empty.
func PostAnnounce(ctx context.Context, req AnnounceRequest) error {
	ep := AnnounceEndpoint()
	if ep == "" {
		return fmt.Errorf("no announce endpoint (set RE_DIRECTORY_ANNOUNCE)")
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, ep, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("announce %s: %s", ep, res.Status)
	}
	return nil
}
