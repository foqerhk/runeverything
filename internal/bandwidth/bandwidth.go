package bandwidth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// Estimate holds measured uplink capacity and derived max sessions.
type Estimate struct {
	MbpsUp      float64   `json:"mbps_up"`
	MbpsDown    float64   `json:"mbps_down,omitempty"`
	MaxSessions int       `json:"max_sessions"`
	PerUserKbps int       `json:"per_user_kbps"`
	MeasuredAt  time.Time `json:"measured_at"`
}

const defaultPerUserKbps = 2500 // ~2.5 Mbps per remote-desktop session budget

// MaxSessionsFromMbps reserves headroom and divides by per-user budget.
func MaxSessionsFromMbps(mbpsUp float64, perUserKbps int) int {
	if perUserKbps <= 0 {
		perUserKbps = defaultPerUserKbps
	}
	if mbpsUp <= 0 {
		return 1
	}
	usable := mbpsUp * 0.7 // 30% headroom for OS / signaling / bursts
	n := int(usable * 1000 / float64(perUserKbps))
	if n < 1 {
		n = 1
	}
	if n > 64 {
		n = 64
	}
	return n
}

// LoadFromEnv reads RE_MAX_SESSIONS or RE_BW_MBPS_UP.
func LoadFromEnv() Estimate {
	e := Estimate{PerUserKbps: defaultPerUserKbps, MeasuredAt: time.Now().UTC()}
	if v := os.Getenv("RE_PER_USER_KBPS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			e.PerUserKbps = n
		}
	}
	if v := os.Getenv("RE_MAX_SESSIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			e.MaxSessions = n
			return e
		}
	}
	if v := os.Getenv("RE_BW_MBPS_UP"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			e.MbpsUp = f
			e.MaxSessions = MaxSessionsFromMbps(f, e.PerUserKbps)
			return e
		}
	}
	e.MaxSessions = 4 // safe default until probed
	e.MbpsUp = 10
	return e
}

// Limiter tracks active desktop/PTY bindings against MaxSessions.
type Limiter struct {
	mu    sync.Mutex
	max   int
	count int
	est   Estimate
}

func NewLimiter(est Estimate) *Limiter {
	if est.MaxSessions < 1 {
		est.MaxSessions = 1
	}
	return &Limiter{max: est.MaxSessions, est: est}
}

func (l *Limiter) Estimate() Estimate {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.est
}

func (l *Limiter) Active() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.count
}

func (l *Limiter) Max() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.max
}

func (l *Limiter) TryAcquire() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.count >= l.max {
		return false
	}
	l.count++
	return true
}

func (l *Limiter) Release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.count > 0 {
		l.count--
	}
}

// BusyScore 0..100 for peer advertisement (higher = busier).
func (l *Limiter) BusyScore() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.max <= 0 {
		return 100
	}
	return (l.count * 100) / l.max
}

// MountHealthExtra adds bandwidth JSON on /v1/capacity
func MountCapacity(mux *http.ServeMux, lim *Limiter) {
	mux.HandleFunc("/v1/capacity", func(w http.ResponseWriter, r *http.Request) {
		est := lim.Estimate()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"mbps_up":      est.MbpsUp,
			"max_sessions": lim.Max(),
			"active":       lim.Active(),
			"busy":         lim.BusyScore(),
			"per_user_kbps": est.PerUserKbps,
		})
	})
}

// ProbeHTTP uploads to a URL for rough uplink estimate (optional external echo).
func ProbeHTTP(ctx context.Context, uploadURL string, bytes int) (mbps float64, err error) {
	if bytes < 256*1024 {
		bytes = 256 * 1024
	}
	body := make([]byte, bytes)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, nil)
	if err != nil {
		return 0, err
	}
	_ = body
	_ = req
	return 0, fmt.Errorf("use scripts/probe-bandwidth.sh for live measurement")
}
