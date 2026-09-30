//go:build windows

package main

import (
	"log"
	"os"

	"github.com/foqerhk/runeverything/internal/i18n"
	"github.com/foqerhk/runeverything/internal/identity"
	"github.com/foqerhk/runeverything/internal/winutil"
	"github.com/getlantern/systray"
)

func cmdTray() {
	release, err := acquireAgentLock()
	if err != nil {
		log.Fatal(err)
	}
	defer release()
	winutil.FreeConsole()
	systray.Run(onTrayReady, onTrayExit)
}

func onTrayExit() {}

func onTrayReady() {
	systray.SetIcon(trayIconICO)
	systray.SetTitle("")
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
		// Pair offer runs inside runLoopRE2 after connect (re2Conn not ready yet).
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
					winutil.NotifyBalloon("RunEverything", i18n.T("tray.autostart_off"))
				} else {
					if err := winutil.RegisterLogonTask(exe); err != nil {
						winutil.NotifyBalloon("RunEverything", i18n.T("tray.autostart_fail", err.Error()))
						continue
					}
					mAuto.Check()
					winutil.NotifyBalloon("RunEverything", i18n.T("tray.autostart_on"))
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
					systray.SetTooltip(i18n.T("tray.tooltip_reconnecting"))
				}
			}
		}
	}()
}
