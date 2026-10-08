package desktop

import (
	"sync"
	"time"
)

// ABRConfig holds adaptive bitrate / FPS targets and an optional discrete
// resolution ladder (client quality tiers: ultra → high → balanced → smooth).
type ABRConfig struct {
	MinFPS      int
	MaxFPS      int
	MinWidth    int
	MaxWidth    int
	MinHeight   int
	MaxHeight   int
	MinBitrateK int
	MaxBitrateK int
	// NativeWidth/Height are the selected display pixels. Ladder scales apply to
	// native (超清=1.0 … 流畅=0.35), then clamp to MaxWidth×MaxHeight (OPEN cap).
	// Empty native → scales apply to Max (legacy / tests).
	NativeWidth  int
	NativeHeight int
	// Ladder is scale factors relative to native (or Max when native unset), highest first.
	// Empty → legacy fractional shrink (tests). Production fills CN tiers.
	Ladder []float64
}

func DefaultABR() ABRConfig {
	return ABRConfig{
		MinFPS: 5, MaxFPS: 30,
		MinWidth: 640, MaxWidth: 15360, MinHeight: 360, MaxHeight: 8640,
		MinBitrateK: 300, MaxBitrateK: 300000,
		// Match KoKo DesktopVideoQuality scaleFactor: 超清/高清/标清/流畅.
		Ladder: []float64{1.0, 0.75, 0.55, 0.35},
	}
}

// StatsSample is one client STATS window (weak-net feedback).
// Legacy callers only fill RTT/Loss/WantKey; newer clients add jitter/kbps/stall.
type StatsSample struct {
	RTTMs         int
	LossPct       float64
	WantKeyframe  bool
	RecvKbps      int
	JitterMs      int
	DecodeDelayMs int
	Stall         bool
	StreamFPS     float64
}

// ABRController adjusts encode params from client Stats feedback.
type ABRController struct {
	mu        sync.Mutex
	cfg       ABRConfig
	fps       int
	width     int
	height    int
	bitrateK  int
	wantKey   bool
	lastAdj   time.Time
	lastRTT   int
	lastLoss  float64
	lastJit   int
	lastKbps  int
	cutStreak int // consecutive "bad/worse" windows — accelerate cut
	rung      int // index into cfg.Ladder (0 = highest)
	// holdUntil blocks rung/bitrate cuts after a fresh OPEN so the client can
	// paint the negotiated tier before weak-net STATS yank us to 流畅.
	holdUntil time.Time
}

func NewABR(cfg ABRConfig, width, height, fps, bitrateK int) *ABRController {
	if fps <= 0 {
		fps = 15
	}
	if bitrateK <= 0 {
		bitrateK = 2500
	}
	a := &ABRController{cfg: cfg, fps: fps, width: width, height: height, bitrateK: bitrateK}
	a.trimLadderToOpen()
	a.rung = a.nearestRung(width, height)
	if len(a.cfg.Ladder) > 0 {
		a.width, a.height = a.sizeForRung(a.rung)
	}
	a.width &^= 1
	a.height &^= 1
	// Lock pixels to the OPEN rung. Bitrate/FPS still adapt; silent mid-stream
	// VT size swaps froze PreferDirect (READY≠pic, then 0 kb/s). Resolution
	// changes only via OPEN / soft reopen (client quality menu or step-down).
	a.lockOpenPixels()
	// Hold the OPEN rung long enough for multipart IDR + first paint (WSS/UDP),
	// and for the client quality E2E window before fake weak-net STATS cut us down.
	a.holdUntil = time.Now().Add(5 * time.Second)
	return a
}

func (a *ABRController) Snapshot() (w, h, fps, bitrateK int, forceKey bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	fk := a.wantKey
	a.wantKey = false
	return a.width, a.height, a.fps, a.bitrateK, fk
}

// Rung returns the current ladder index (0 = highest / 超清).
func (a *ABRController) Rung() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.rung
}

func (a *ABRController) RequestKeyframe() {
	a.mu.Lock()
	a.wantKey = true
	a.mu.Unlock()
}

// ApplyOpenTarget hot-swaps the OPEN ladder without tearing capture down.
// Used for mid-session quality menu reopens (full close→open was peer_gone on UDP·LAN).
func (a *ABRController) ApplyOpenTarget(width, height, fps, bitrateK int, cfg ABRConfig) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg = cfg
	if width > 0 {
		a.cfg.MaxWidth = width
	}
	if height > 0 {
		a.cfg.MaxHeight = height
	}
	if bitrateK > 0 {
		a.cfg.MaxBitrateK = bitrateK
		a.bitrateK = bitrateK
	}
	if fps > 0 {
		a.cfg.MaxFPS = fps
		a.fps = fps
	}
	a.trimLadderToOpen()
	a.rung = a.nearestRung(width, height)
	if len(a.cfg.Ladder) > 0 {
		a.width, a.height = a.sizeForRung(a.rung)
	} else {
		a.width, a.height = width, height
	}
	a.width &^= 1
	a.height &^= 1
	a.lockOpenPixels()
	a.holdUntil = time.Now().Add(5 * time.Second)
	a.wantKey = true
	a.cutStreak = 0
	a.lastAdj = time.Time{}
}

// lockOpenPixels pins Min/Max to the current encode size so OnStats cannot
// step the discrete ladder (or fractional shrink) without a new OPEN.
func (a *ABRController) lockOpenPixels() {
	if a.width > 0 {
		a.cfg.MinWidth = a.width
		a.cfg.MaxWidth = a.width
	}
	if a.height > 0 {
		a.cfg.MinHeight = a.height
		a.cfg.MaxHeight = a.height
	}
}

// LastNetwork returns the most recent STATS sample (rtt ms, loss %).
func (a *ABRController) LastNetwork() (rttMs int, lossPct float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastRTT, a.lastLoss
}

// OnStats updates targets from legacy RTT/loss-only feedback.
func (a *ABRController) OnStats(rttMs int, lossPct float64, wantKey bool) {
	a.OnStatsSample(StatsSample{RTTMs: rttMs, LossPct: lossPct, WantKeyframe: wantKey})
}

// OnStatsSample is the weak-net path: uses jitter / recv_kbps / stall when present.
func (a *ABRController) OnStatsSample(s StatsSample) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastRTT = s.RTTMs
	a.lastLoss = s.LossPct
	a.lastJit = s.JitterMs
	a.lastKbps = s.RecvKbps
	if s.WantKeyframe || s.Stall {
		a.wantKey = true
	}
	now := time.Now()
	// Fresh OPEN: accept keyframe asks but do not cut rungs/bitrate yet.
	if now.Before(a.holdUntil) {
		return
	}
	// Stall / severe loss: allow faster reaction than the 500ms floor.
	minGap := 500 * time.Millisecond
	if s.Stall || s.LossPct > 12 {
		minGap = 250 * time.Millisecond
	}
	if now.Sub(a.lastAdj) < minGap {
		return
	}
	a.lastAdj = now

	// RTT affects interaction latency, not link capacity. Frame-arrival jitter is
	// also not a bandwidth signal for dirty-rectangle capture: a healthy static
	// desktop naturally emits at irregular intervals. Use measured loss/stall.
	effLoss := s.LossPct
	if s.Stall && effLoss < 8 {
		effLoss = 8
	}

	// A static desktop receives far less than the configured bitrate because there
	// are few changed frames. Compare receive rate to target only while enough
	// frames are actually being produced to create sustained bandwidth demand.
	bwPressure := false
	minDemandFPS := float64(a.fps) * 0.6
	if minDemandFPS < 5 {
		minDemandFPS = 5
	}
	if s.StreamFPS >= minDemandFPS && s.RecvKbps > 0 && a.bitrateK > 800 {
		if s.RecvKbps < a.bitrateK*7/10 {
			bwPressure = true
		}
	}

	worse := effLoss > 12 || s.Stall || (bwPressure && effLoss > 3)
	bad := effLoss > 5 || bwPressure
	good := effLoss < 1 && !s.Stall && !bwPressure

	resLocked := a.cfg.MaxWidth > 0 && a.cfg.MinWidth >= a.cfg.MaxWidth &&
		a.cfg.MaxHeight > 0 && a.cfg.MinHeight >= a.cfg.MaxHeight
	useLadder := !resLocked && len(a.cfg.Ladder) > 0

	if worse {
		a.cutStreak++
		factor := 6 // /10
		if a.cutStreak >= 3 {
			factor = 5
		}
		a.bitrateK = max(a.cfg.MinBitrateK, a.bitrateK*factor/10)
		a.capBitrateToReceive(s.RecvKbps)
		a.fps = max(a.cfg.MinFPS, a.fps-5)
		if useLadder {
			a.stepDownRung()
		} else if !resLocked {
			a.shrinkRes(3, 4)
		}
		a.wantKey = true
	} else if bad {
		a.cutStreak++
		a.bitrateK = max(a.cfg.MinBitrateK, a.bitrateK*8/10)
		a.capBitrateToReceive(s.RecvKbps)
		a.fps = max(a.cfg.MinFPS, a.fps-2)
		// Only drop a quality rung after sustained bad windows (not one blip).
		if a.cutStreak >= 2 && (effLoss > 8 || s.JitterMs > 70) {
			if useLadder {
				a.stepDownRung()
			} else if !resLocked {
				a.shrinkRes(7, 8)
			}
		}
	} else if good {
		a.cutStreak = 0
		a.bitrateK = min(a.cfg.MaxBitrateK, a.bitrateK*105/100+50)
		a.fps = min(a.cfg.MaxFPS, a.fps+1)
		if useLadder && a.bitrateK > a.cfg.MinBitrateK*2 {
			a.stepUpRung()
		} else if !resLocked && !useLadder && a.width < a.cfg.MaxWidth && a.bitrateK > a.cfg.MinBitrateK*2 {
			a.width = min(a.cfg.MaxWidth, a.width+40)
			maxH := a.cfg.MaxHeight
			if maxH <= 0 {
				maxH = a.cfg.MaxWidth * 9 / 16
			}
			a.height = min(maxH, a.height+24)
		}
	} else {
		if a.cutStreak > 0 {
			a.cutStreak--
		}
	}
	if resLocked {
		a.width = a.cfg.MaxWidth
		a.height = a.cfg.MaxHeight
	}
	a.width &^= 1
	a.height &^= 1
}

// capBitrateToReceive closes the loop in one sample after a bandwidth cliff.
// Leaving 15% headroom avoids repeatedly filling relay/carrier queues.
func (a *ABRController) capBitrateToReceive(recvKbps int) {
	if recvKbps <= 0 {
		return
	}
	target := recvKbps * 85 / 100
	if target < a.cfg.MinBitrateK {
		target = a.cfg.MinBitrateK
	}
	if a.bitrateK > target {
		a.bitrateK = target
	}
}

func (a *ABRController) stepDownRung() {
	if a.rung+1 >= len(a.cfg.Ladder) {
		return
	}
	a.rung++
	a.width, a.height = a.sizeForRung(a.rung)
	a.applyRungBitrate()
	a.wantKey = true
}

func (a *ABRController) stepUpRung() {
	if a.rung <= 0 {
		return
	}
	a.rung--
	a.width, a.height = a.sizeForRung(a.rung)
	a.applyRungBitrate()
	a.wantKey = true
}

// applyRungBitrate sets bitrate from MaxBitrateK × (scale/topScale)² so each
// discrete rung carries a matching kb/s relative to the OPEN tier budget.
func (a *ABRController) applyRungBitrate() {
	if len(a.cfg.Ladder) == 0 || a.cfg.MaxBitrateK <= 0 {
		return
	}
	top := a.cfg.Ladder[0]
	if top <= 0 {
		return
	}
	scale := a.cfg.Ladder[a.rung]
	rel := scale / top
	br := int(float64(a.cfg.MaxBitrateK)*rel*rel + 0.5)
	a.bitrateK = max(a.cfg.MinBitrateK, min(a.cfg.MaxBitrateK, br))
}

func (a *ABRController) sizeForRung(rung int) (w, h int) {
	if len(a.cfg.Ladder) == 0 {
		return a.width, a.height
	}
	if rung < 0 {
		rung = 0
	}
	if rung >= len(a.cfg.Ladder) {
		rung = len(a.cfg.Ladder) - 1
	}
	scale := a.cfg.Ladder[rung]
	// Prefer native display as the ladder reference so mid-tier OPEN
	// (e.g. 标清) steps to 流畅 = 0.35×native — never 0.35×OPEN (invented size).
	refW := a.cfg.NativeWidth
	refH := a.cfg.NativeHeight
	if refW <= 0 {
		refW = a.cfg.MaxWidth
	}
	if refH <= 0 {
		refH = a.cfg.MaxHeight
	}
	if refW <= 0 {
		refW = a.width
	}
	if refH <= 0 {
		refH = a.height
	}
	w = int(float64(refW)*scale + 0.5)
	h = int(float64(refH)*scale + 0.5)
	maxW := a.cfg.MaxWidth
	maxH := a.cfg.MaxHeight
	if a.cfg.MinWidth > 0 && w < a.cfg.MinWidth {
		w = a.cfg.MinWidth
	}
	if a.cfg.MinHeight > 0 && h < a.cfg.MinHeight {
		h = a.cfg.MinHeight
	}
	if maxW > 0 && w > maxW {
		w = maxW
	}
	if maxH > 0 && h > maxH {
		h = maxH
	}
	return w &^ 1, h &^ 1
}

// trimLadderToOpen drops rungs whose native-scaled size exceeds OPEN max, so
// the top rung matches the negotiated quality (超清/高清/标清/流畅).
func (a *ABRController) trimLadderToOpen() {
	if len(a.cfg.Ladder) == 0 || a.cfg.MaxWidth <= 0 || a.cfg.MaxHeight <= 0 {
		return
	}
	refW := a.cfg.NativeWidth
	if refW <= 0 {
		refW = a.cfg.MaxWidth
	}
	var kept []float64
	for _, s := range a.cfg.Ladder {
		w := int(float64(refW)*s + 0.5)
		// Gate on width only. Height-gated trim dropped 标清 when native was
		// taller than the OPEN box (1920x1200×0.55→1056x660 > maxH 594) and
		// left only 流畅 — encode at 672 while READY stayed ~950.
		// sizeForRung still clamps both dims to MaxWidth/MaxHeight.
		if w <= a.cfg.MaxWidth+2 {
			kept = append(kept, s)
		}
	}
	if len(kept) == 0 {
		kept = []float64{a.cfg.Ladder[len(a.cfg.Ladder)-1]}
	}
	a.cfg.Ladder = kept
}

func (a *ABRController) nearestRung(width, height int) int {
	if len(a.cfg.Ladder) == 0 {
		return 0
	}
	best, bestDist := 0, int(^uint(0)>>1)
	for i := range a.cfg.Ladder {
		rw, rh := a.sizeForRung(i)
		d := absInt(rw-width) + absInt(rh-height)
		if d < bestDist {
			bestDist = d
			best = i
		}
	}
	return best
}

func (a *ABRController) shrinkRes(num, den int) {
	minW := a.cfg.MinWidth
	if a.cfg.MaxWidth > 0 && minW > a.cfg.MaxWidth {
		minW = a.cfg.MaxWidth
	}
	a.width = max(minW, a.width*num/den)
	if a.cfg.MaxWidth > 0 {
		a.width = min(a.cfg.MaxWidth, a.width)
	}
	minH := a.cfg.MinHeight
	if minH <= 0 {
		minH = minW * 9 / 16
	}
	if a.cfg.MaxHeight > 0 && minH > a.cfg.MaxHeight {
		minH = a.cfg.MaxHeight
	}
	a.height = max(minH, a.height*num/den)
	if a.cfg.MaxHeight > 0 {
		a.height = min(a.cfg.MaxHeight, a.height)
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
