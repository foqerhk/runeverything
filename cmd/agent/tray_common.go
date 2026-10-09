package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/foqerhk/runeverything/internal/desktop"
	"github.com/foqerhk/runeverything/internal/identity"
	"github.com/foqerhk/runeverything/internal/keepalive"
	"github.com/foqerhk/runeverything/internal/pairing"
	"github.com/foqerhk/runeverything/internal/protocol"
	ptyx "github.com/foqerhk/runeverything/internal/pty"
)

var (
	trayAgentMu sync.RWMutex
	trayAgent   *Agent
)

func setTrayAgent(a *Agent) {
	trayAgentMu.Lock()
	trayAgent = a
	trayAgentMu.Unlock()
}

func getTrayAgent() *Agent {
	trayAgentMu.RLock()
	defer trayAgentMu.RUnlock()
	return trayAgent
}

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
	discovered, alts := applyRelayDiscovery(cfg, "")
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
		sessionIdle: envDuration("RE_SESSION_IDLE", 2*time.Minute),
		altRelays:   alts,
	}
	if onDesktop != nil {
		a.onDesktopChange = onDesktop
	}
	setTrayAgent(a)
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
	path, link, _, err := mintPairQR(a)
	return path, link, err
}

// mintPairQR rotates the pairing token, writes PNG, and returns path/link/expiry.
func mintPairQR(a *Agent) (pngPath, deepLink string, expiresAt int64, err error) {
	if err := a.offerPairRE2(false, true); err != nil {
		return "", "", 0, err
	}
	p, err := loadLastPairing()
	if err != nil {
		return "", "", 0, err
	}
	home, _ := identity.HomeDir()
	path, err := pairing.WriteQRPNG(p, home)
	if err != nil {
		return "", "", 0, err
	}
	return path, p.DeepLink(), p.ExpiresAt, nil
}

// encodePairQRPNG mints a fresh pairing QR and returns PNG bytes (no Preview).
func encodePairQRPNG(a *Agent) (png []byte, deepLink string, expiresAt int64, err error) {
	if err := a.offerPairRE2(false, true); err != nil {
		return nil, "", 0, err
	}
	p, err := loadLastPairing()
	if err != nil {
		return nil, "", 0, err
	}
	home, _ := identity.HomeDir()
	_, _ = pairing.WriteQRPNG(p, home) // keep last file in sync for "open folder"
	png, err = pairing.EncodeQRPNG(p, 512)
	if err != nil {
		return nil, "", 0, err
	}
	return png, p.DeepLink(), p.ExpiresAt, nil
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
