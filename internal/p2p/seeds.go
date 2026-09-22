package p2p

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

	"github.com/foqerhk/runeverything/internal/region"
)

const (
	DefaultGossipInterval = 30 * time.Second
	DefaultPeerTTL        = 15 * time.Minute
	MaxCrawlPeers         = 64
	MaxCrawlDepth         = 3
)

// SeedsFile is the JSON shape of the official bootstrap list.
type SeedsFile struct {
	Seeds []json.RawMessage `json:"seeds"`
}

func parseSeedEntry(raw json.RawMessage) (urlStr string, ok bool) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s), s != ""
	}
	var obj struct {
		URL string `json:"url"`
		UDP string `json:"udp"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && obj.URL != "" {
		return strings.TrimSpace(obj.URL), true
	}
	return "", false
}

// SeedURLs returns bootstrap relay URLs (env RE_SEEDS comma-list overrides file URL).
// Official lists are region-scoped: CN → getnode.intentcomputing.cn only;
// intl → getnode.intentcomputing.net (plus optional GitHub mirrors). No cross-border fallback.
func SeedURLs(ctx context.Context) ([]string, error) {
	if v := strings.TrimSpace(os.Getenv("RE_SEEDS")); v != "" {
		parts := strings.Split(v, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			nu, err := NormalizeRelayURL(p)
			if err != nil {
				continue
			}
			out = append(out, nu)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("RE_SEEDS set but empty/invalid")
		}
		return out, nil
	}
	urls := region.SeedsURLs(ctx)
	var last error
	for _, u := range urls {
		seeds, err := fetchSeedsFile(ctx, u)
		if err != nil {
			last = err
			continue
		}
		if len(seeds) > 0 {
			return seeds, nil
		}
	}
	if last != nil {
		return nil, last
	}
	return nil, fmt.Errorf("no seeds available (region=%s)", region.Detect(ctx))
}

func fetchSeedsFile(ctx context.Context, fileURL string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 8 * time.Second}
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
		return nil, fmt.Errorf("%s: %s", fileURL, res.Status)
	}
	body = bytes.TrimSpace(body)
	var sf SeedsFile
	if err := json.Unmarshal(body, &sf); err != nil {
		var arr []json.RawMessage
		if err2 := json.Unmarshal(body, &arr); err2 != nil {
			return nil, err
		}
		sf.Seeds = arr
	}
	out := make([]string, 0, len(sf.Seeds))
	seen := map[string]struct{}{}
	for _, raw := range sf.Seeds {
		s, ok := parseSeedEntry(raw)
		if !ok {
			continue
		}
		nu, err := NormalizeRelayURL(s)
		if err != nil {
			continue
		}
		if _, ok := seen[nu]; ok {
			continue
		}
		seen[nu] = struct{}{}
		out = append(out, nu)
	}
	return out, nil
}
