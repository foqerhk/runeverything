package desktop

import (
	"testing"
	"time"
)

func forceAdj(a *ABRController) {
	a.mu.Lock()
	a.lastAdj = time.Time{}
	a.holdUntil = time.Time{} // expire post-OPEN hold so cuts apply in unit tests
	a.mu.Unlock()
}

func TestABR_HoldBlocksCutAfterOpen(t *testing.T) {
	cfg := DefaultABR()
	cfg.Ladder = nil
	a := NewABR(cfg, 1920, 1080, 30, 8000)
	a.mu.Lock()
	a.lastAdj = time.Time{}
	a.mu.Unlock()
	a.OnStatsSample(StatsSample{RTTMs: 400, LossPct: 15, Stall: true})
	_, _, _, br, _ := a.Snapshot()
	if br != 8000 {
		t.Fatalf("hold should block cut, br=%d", br)
	}
	forceAdj(a)
	a.OnStatsSample(StatsSample{RTTMs: 400, LossPct: 15, Stall: true})
	_, _, _, br2, _ := a.Snapshot()
	if br2 >= 8000 {
		t.Fatalf("after hold release should cut, br=%d", br2)
	}
}

func TestABR_WorseCutsHard(t *testing.T) {
	cfg := DefaultABR()
	cfg.Ladder = nil // exercise legacy fractional shrink
	a := NewABR(cfg, 1920, 1080, 30, 8000)
	// Unlock pixels — production locks OPEN size; this test covers legacy shrink.
	a.mu.Lock()
	a.cfg.MinWidth, a.cfg.MinHeight = 640, 360
	a.cfg.MaxWidth, a.cfg.MaxHeight = 1920, 1080
	a.mu.Unlock()
	forceAdj(a)
	a.OnStatsSample(StatsSample{RTTMs: 400, LossPct: 15, Stall: true})
	w, h, fps, br, key := a.Snapshot()
	if br >= 8000 {
		t.Fatalf("bitrate not cut: %d", br)
	}
	if fps >= 30 {
		t.Fatalf("fps not cut: %d", fps)
	}
	if w >= 1920 || h >= 1080 {
		t.Fatalf("res not shrunk: %dx%d", w, h)
	}
	if !key {
		t.Fatal("expected keyframe after worse")
	}
}

func TestABR_LadderStepsDiscrete(t *testing.T) {
	cfg := DefaultABR()
	cfg.NativeWidth, cfg.NativeHeight = 1920, 1080
	cfg.MaxWidth, cfg.MaxHeight = 1920, 1080
	cfg.MinWidth, cfg.MinHeight = 672, 378
	cfg.MaxBitrateK = 8000
	a := NewABR(cfg, 1920, 1080, 30, 8000)
	// Production locks OPEN pixels; unlock so this unit test can step the ladder.
	a.mu.Lock()
	a.cfg.MinWidth, a.cfg.MinHeight = 672, 378
	a.cfg.MaxWidth, a.cfg.MaxHeight = 1920, 1080
	a.mu.Unlock()
	forceAdj(a)
	a.OnStatsSample(StatsSample{RTTMs: 400, LossPct: 15, Stall: true})
	w, h, _, br, _ := a.Snapshot()
	// One step: 超清 → 高清 (0.75); bitrate ≈ MaxBR × (0.75/1)²
	if w != 1440 || h != 810 {
		t.Fatalf("expected discrete high rung 1440x810, got %dx%d", w, h)
	}
	wantBR := int(8000 * 0.75 * 0.75)
	if br != wantBR {
		t.Fatalf("high rung bitrate want %d got %d", wantBR, br)
	}
	forceAdj(a)
	a.OnStatsSample(StatsSample{RTTMs: 400, LossPct: 15, Stall: true})
	w, h, _, br, _ = a.Snapshot()
	// Next: 标清 (0.55)
	if w != 1056 || h != 594 {
		t.Fatalf("expected discrete balanced rung 1056x594, got %dx%d", w, h)
	}
	wantBR = int(8000 * 0.55 * 0.55)
	if br != wantBR {
		t.Fatalf("balanced rung bitrate want %d got %d", wantBR, br)
	}
}

func TestABR_LadderFromBalancedOpen(t *testing.T) {
	// OPEN already at 标清 — next worse step must be 流畅 (0.35×native), not 0.35×OPEN.
	cfg := DefaultABR()
	cfg.NativeWidth, cfg.NativeHeight = 1920, 1080
	cfg.MaxWidth, cfg.MaxHeight = 1056, 594
	cfg.MinWidth, cfg.MinHeight = 672, 378
	cfg.MaxBitrateK = 2000
	a := NewABR(cfg, 1056, 594, 24, 2000)
	a.mu.Lock()
	a.cfg.MinWidth, a.cfg.MinHeight = 672, 378
	a.cfg.MaxWidth, a.cfg.MaxHeight = 1056, 594
	a.mu.Unlock()
	forceAdj(a)
	a.OnStatsSample(StatsSample{RTTMs: 400, LossPct: 15, Stall: true})
	w, h, _, _, _ := a.Snapshot()
	if w != 672 || h != 378 {
		t.Fatalf("expected smooth rung 672x378 from balanced OPEN, got %dx%d", w, h)
	}
}

func TestABR_OpenLocksResolution(t *testing.T) {
	cfg := DefaultABR()
	cfg.NativeWidth, cfg.NativeHeight = 1920, 1080
	a := NewABR(cfg, 1056, 594, 24, 2000)
	forceAdj(a)
	a.OnStatsSample(StatsSample{RTTMs: 500, LossPct: 20, Stall: true})
	w, h, _, br, _ := a.Snapshot()
	if w != 1056 || h != 594 {
		t.Fatalf("OPEN pixels must stay locked, got %dx%d", w, h)
	}
	if br >= 2000 {
		t.Fatalf("bitrate should still cut when locked: %d", br)
	}
}

func TestABR_DirtyFrameJitterDoesNotCut(t *testing.T) {
	a := NewABR(DefaultABR(), 1280, 720, 24, 4000)
	forceAdj(a)
	// Irregular dirty-frame arrivals are normal on a static desktop.
	a.OnStatsSample(StatsSample{RTTMs: 120, LossPct: 0.5, JitterMs: 90})
	_, _, _, br, _ := a.Snapshot()
	if br < 4000 {
		t.Fatalf("jitter alone should not pressure bitrate, got %d", br)
	}
}

func TestABR_HighRTTDoesNotCutCapacity(t *testing.T) {
	a := NewABR(DefaultABR(), 1280, 720, 24, 4000)
	forceAdj(a)
	a.OnStatsSample(StatsSample{RTTMs: 450, LossPct: 0.2, JitterMs: 100})
	_, _, _, br, _ := a.Snapshot()
	if br < 4000 {
		t.Fatalf("RTT alone should not pressure bitrate, got %d", br)
	}
}

func TestABR_StaticLowReceiveDoesNotCut(t *testing.T) {
	a := NewABR(DefaultABR(), 1920, 1080, 30, 10000)
	forceAdj(a)
	// Low receive rate with few changed frames is low demand, not low capacity.
	a.OnStatsSample(StatsSample{
		RTTMs: 150, LossPct: 0.2, RecvKbps: 300, StreamFPS: 1,
	})
	_, _, _, br, _ := a.Snapshot()
	if br < 10000 {
		t.Fatalf("static desktop should not pressure bitrate, got %d", br)
	}
}

func TestABR_GoodRampsSlowly(t *testing.T) {
	cfg := DefaultABR()
	cfg.MaxWidth, cfg.MaxHeight = 1280, 720
	cfg.MaxBitrateK = 8000
	a := NewABR(cfg, 1280, 720, 20, 2000)
	forceAdj(a)
	a.OnStatsSample(StatsSample{RTTMs: 40, LossPct: 0.2, JitterMs: 10, RecvKbps: 2500})
	_, _, fps, br, _ := a.Snapshot()
	if br <= 2000 {
		t.Fatalf("expected gentle ramp, br=%d", br)
	}
	if br > 2000*105/100+50+1 {
		t.Fatalf("ramp too aggressive: %d", br)
	}
	if fps < 20 || fps > 21 {
		t.Fatalf("fps step unexpected: %d", fps)
	}
}

func TestABR_BwPressureCuts(t *testing.T) {
	a := NewABR(DefaultABR(), 1920, 1080, 30, 10000)
	forceAdj(a)
	// Client only receiving 2 Mbps while we encode at 10 Mbps.
	a.OnStatsSample(StatsSample{
		RTTMs: 150, LossPct: 1, RecvKbps: 2000, StreamFPS: 30,
	})
	_, _, _, br, _ := a.Snapshot()
	if br >= 10000 {
		t.Fatalf("bw pressure should cut bitrate, got %d", br)
	}
}

func TestABR_BwPressureLeavesReceiveHeadroom(t *testing.T) {
	a := NewABR(DefaultABR(), 1920, 1080, 30, 10000)
	forceAdj(a)
	a.OnStatsSample(StatsSample{
		RTTMs: 150, LossPct: 1, RecvKbps: 2000, StreamFPS: 30,
	})
	_, _, _, br, _ := a.Snapshot()
	if br > 1700 {
		t.Fatalf("bitrate should leave 15%% receive headroom, got %d", br)
	}
}

func TestABR_ResLockedIgnoresShrink(t *testing.T) {
	cfg := DefaultABR()
	cfg.MinWidth, cfg.MaxWidth = 1920, 1920
	cfg.MinHeight, cfg.MaxHeight = 1080, 1080
	a := NewABR(cfg, 1920, 1080, 30, 8000)
	forceAdj(a)
	a.OnStatsSample(StatsSample{RTTMs: 500, LossPct: 20, Stall: true})
	w, h, _, br, _ := a.Snapshot()
	if w != 1920 || h != 1080 {
		t.Fatalf("locked res changed: %dx%d", w, h)
	}
	if br >= 8000 {
		t.Fatalf("bitrate should still cut when locked: %d", br)
	}
}

func TestABR_LegacyOnStatsStillWorks(t *testing.T) {
	a := NewABR(DefaultABR(), 1280, 720, 30, 5000)
	forceAdj(a)
	a.OnStats(200, 6, false)
	_, _, _, br, _ := a.Snapshot()
	if br >= 5000 {
		t.Fatalf("legacy OnStats should cut on bad: %d", br)
	}
}
