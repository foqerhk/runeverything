// Package region selects China vs international official endpoints.
// Policy: once classified, the client talks only to that region's getnode
// (no cross-border fallback), so volunteer enroll/claim/report and seed
// bootstrap stay in-region.
package region

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	CN   = "cn"
	Intl = "intl"

	// GetnodeCN is the mainland China registrar + seeds host.
	GetnodeCN = "https://getnode.intentcomputing.cn"
	// GetnodeIntl is the international registrar + seeds host.
	GetnodeIntl = "https://getnode.intentcomputing.net"
)

var (
	detectOnce sync.Once
	detected   string
)

// Override returns RE_REGION if set to cn/intl.
func Override() string {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("RE_REGION")))
	switch v {
	case CN, "china", "mainland":
		return CN
	case Intl, "global", "overseas", "international":
		return Intl
	default:
		return ""
	}
}

// Detect returns cn or intl. Result is cached for the process lifetime.
func Detect(ctx context.Context) string {
	detectOnce.Do(func() {
		if o := Override(); o != "" {
			detected = o
			return
		}
		if looksLikeMainlandLocal() {
			detected = CN
			return
		}
		if cn, err := lookupIPCountry(ctx); err == nil {
			if cn {
				detected = CN
				return
			}
			detected = Intl
			return
		}
		// Fail-open to international when geo is unknown (typical overseas).
		detected = Intl
	})
	return detected
}

func looksLikeMainlandLocal() bool {
	candidates := []string{
		os.Getenv("TZ"),
		os.Getenv("LANG"),
		os.Getenv("LC_ALL"),
		os.Getenv("LC_MESSAGES"),
	}
	if name, off := time.Now().Zone(); name != "" {
		candidates = append(candidates, name)
		_ = off
	}
	blob := strings.ToLower(strings.Join(candidates, " "))
	for _, p := range []string{
		"asia/shanghai", "asia/chongqing", "asia/urumqi", "asia/harbin",
		"asia/beijing", "prc", "zh_cn", "zh-cn", "zh_hans",
	} {
		if strings.Contains(blob, p) {
			return true
		}
	}
	return false
}

func lookupIPCountry(ctx context.Context) (isCN bool, err error) {
	// Public IP via a short, fields-limited lookup. Used only for region class.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://ip-api.com/json/?fields=status,countryCode", nil)
	if err != nil {
		return false, err
	}
	client := &http.Client{Timeout: 3 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	var out struct {
		Status      string `json:"status"`
		CountryCode string `json:"countryCode"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return false, err
	}
	if out.Status != "success" {
		return false, net.ErrClosed
	}
	return strings.EqualFold(out.CountryCode, "CN"), nil
}

// GetnodeBase returns the in-region registrar base URL (no trailing slash).
// RE_REGISTRAR_URL overrides when set.
func GetnodeBase(ctx context.Context) string {
	if v := strings.TrimSpace(os.Getenv("RE_REGISTRAR_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	if Detect(ctx) == CN {
		return GetnodeCN
	}
	return GetnodeIntl
}

// SeedsURLs returns bootstrap seed list URLs for this region only.
// RE_SEEDS_URL, if set, is tried first still within the same region policy
// (caller may pass any URL; we do not add the other region's getnode).
func SeedsURLs(ctx context.Context) []string {
	if v := strings.TrimSpace(os.Getenv("RE_SEEDS_URL")); v != "" {
		return []string{v}
	}
	base := GetnodeBase(ctx)
	primary := base + "/seeds.json"
	if Detect(ctx) == CN {
		// Mainland: only CN getnode — no GitHub / overseas mirrors.
		return []string{primary}
	}
	// International: getnode.net, then GitHub Pages / raw as soft mirrors
	// (same geopolitical zone as .net ops, not CN).
	return []string{
		primary,
		"https://foqerhk.github.io/runeverything/seeds.json",
		"https://raw.githubusercontent.com/foqerhk/runeverything/main/docs/seeds.json",
	}
}
