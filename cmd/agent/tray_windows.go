//go:build windows

package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/foqerhk/runeverything/internal/desktop"
	"github.com/foqerhk/runeverything/internal/i18n"
	"github.com/foqerhk/runeverything/internal/identity"
	"github.com/foqerhk/runeverything/internal/keepalive"
	"github.com/foqerhk/runeverything/internal/pairing"
	"github.com/foqerhk/runeverything/internal/protocol"
	ptyx "github.com/foqerhk/runeverything/internal/pty"
	"github.com/foqerhk/runeverything/internal/winutil"
	"github.com/getlantern/systray"
)

func cmdTray() {
	winutil.FreeConsole()
	systray.Run(onTrayReady, onTrayExit)
}

func onTrayExit() {}

func onTrayReady() {
	systray.SetIcon(trayIconICO)
	systray.SetTitle("RunEverything")
	systray.SetTooltip(i18n.T("tray.tooltip"))

	mQR := systray.AddMenuItem(i18n.T("tray.show_qr"), i18n.T("tray.show_qr_tip"))
	mCopy := systray.AddMenuItem(i18n.T("tray.copy_link"), i18n.T("tray.copy_link_tip"))
	mFolder := systray.AddMenuItem(i18n.T("tray.open_folder"), i18n.T("tray.open_folder_tip"))
	systray.AddSeparator()
	mAuto := systray.AddMenuItemCheckbox(i18n.T("tray.start_windows"), i18n.T("tray.start_windows_tip"), winutil.LogonTaskExists())
	systray.AddSeparator()
	mQuit := systray.AddMenuItem(i18n.T("tray.quit"), i18n.T("tray.quit_tip"))

	agent, stopAwake, errCh := startTrayAgent()
	if agent == nil {
		systray.SetTooltip(i18n.T("tray.tooltip_failed"))
	} else {
		systray.SetTooltip(i18n.T("tray.tooltip_running"))
		_ = refreshPairSilent(agent)
	}

	go func() {
		for {
			select {
			case <-mQR.ClickedCh:
				if agent == nil {
					winutil.NotifyBalloon(i18n.T("desktop.confirm_title"), i18n.T("tray.not_running"))
					continue
				}
				path, link, err := showPairQR(agent)
				if err != nil {
					winutil.NotifyBalloon(i18n.T("desktop.confirm_title"), i18n.T("tray.pair_failed", err.Error()))
					continue
				}
				_ = winutil.OpenFile(path)
				_ = winutil.SetClipboardText(link)
				winutil.NotifyBalloon(i18n.T("desktop.confirm_title"), i18n.T("tray.qr_opened"))
			case <-mCopy.ClickedCh:
				if agent == nil {
					continue
				}
				_, link, err := showPairQR(agent)
				if err != nil {
					winutil.NotifyBalloon(i18n.T("desktop.confirm_title"), err.Error())
					continue
				}
				_ = winutil.SetClipboardText(link)
				winutil.NotifyBalloon(i18n.T("desktop.confirm_title"), i18n.T("tray.link_copied"))
			case <-mFolder.ClickedCh:
				home, _ := identity.HomeDir()
				_ = winutil.OpenFolder(home)
			case <-mAuto.ClickedCh:
				exe, _ := os.Executable()
				if winutil.LogonTaskExists() {
					_ = winutil.UnregisterLogonTask()
					mAuto.Uncheck()
					winutil.NotifyBalloon("RunEverything", "Autostart disabled")
				} else {
					if err := winutil.RegisterLogonTask(exe); err != nil {
						winutil.NotifyBalloon("RunEverything", "Autostart failed: "+err.Error())
						continue
					}
					mAuto.Check()
					winutil.NotifyBalloon("RunEverything", "Autostart enabled")
				}
			case <-mQuit.ClickedCh:
				if stopAwake != nil {
					stopAwake()
				}
				if agent != nil {
					agent.closeAll()
				}
				systray.Quit()
				return
			case err := <-errCh:
				if err != nil {
					log.Printf("tray agent: %v", err)
					systray.SetTooltip("RunEverything — reconnecting…")
				}
			}
		}
	}()
}

func startTrayAgent() (*Agent, func(), <-chan error) {
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
	return a.offerPairRE2(false)
}

func showPairQR(a *Agent) (pngPath, deepLink string, err error) {
	if err := a.offerPairRE2(false); err != nil {
		// fall back to last_pairing.json
		p, e2 := loadLastPairing()
		if e2 != nil {
			return "", "", err
		}
		home, _ := identity.HomeDir()
		path, e3 := pairing.WriteQRPNG(p, home)
		if e3 != nil {
			return "", "", e3
		}
		return path, p.DeepLink(), nil
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
