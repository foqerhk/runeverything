package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/foqerhk/runeverything/internal/desktop"
	"github.com/foqerhk/runeverything/internal/identity"
	"github.com/foqerhk/runeverything/internal/keepalive"
	"github.com/foqerhk/runeverything/internal/pairing"
	"github.com/foqerhk/runeverything/internal/protocol"
	ptyx "github.com/foqerhk/runeverything/internal/pty"
)

func startTrayAgent(onDesktop func(active bool)) (*Agent, func(), <-chan error) {
	errCh := make(chan error, 4)
	id, err := identity.LoadOrCreate()
	if err != nil {
		errCh <- err
		return nil, nil, errCh
	}
	cfg, err := identity.LoadConfig()
	if err != nil {
		errCh <- err
		return nil, nil, errCh
	}
	applyNetworkPrefs(cfg)
	discovered := applyRelayDiscovery(cfg, "")
	finalizePublicRelay(cfg, discovered)
	applyRE2Paths(cfg)
	_ = identity.SaveConfig(cfg)

	a := &Agent{
		id:          id,
		cfg:         cfg,
		sessions:    make(map[string]*ptyx.Session),
		printQR:     false,
		clip:        desktop.NewClipboardHub(),
		xferNames:   make(map[string]string),
		sessionIdle: envDuration("RE_SESSION_IDLE", 30*time.Minute),
	}
	if onDesktop != nil {
		a.onDesktopChange = onDesktop
	}
	stopAwake := keepalive.Start()
	go func() {
		backoff := time.Second
		for {
			err := a.runLoopRE2()
			select {
			case errCh <- err:
			default:
			}
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
		}
	}()
	return a, stopAwake, errCh
}

func refreshPairSilent(a *Agent) error {
	return a.offerPairRE2(false, true)
}

func showPairQR(a *Agent) (pngPath, deepLink string, err error) {
	// Mint/refresh so the shown code matches what the relay currently holds.
	if err := a.offerPairRE2(false, true); err != nil {
		return "", "", err
	}
	p, err := loadLastPairing()
	if err != nil {
		return "", "", err
	}
	home, _ := identity.HomeDir()
	path, err := pairing.WriteQRPNG(p, home)
	if err != nil {
		return "", "", err
	}
	return path, p.DeepLink(), nil
}

func loadLastPairing() (*protocol.PairingPayload, error) {
	home, err := identity.HomeDir()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(home, "last_pairing.json"))
	if err != nil {
		return nil, err
	}
	var p protocol.PairingPayload
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
