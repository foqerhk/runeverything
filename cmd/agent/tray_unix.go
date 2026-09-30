//go:build darwin || linux

package main

import (
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/foqerhk/runeverything/internal/desktop"
	"github.com/foqerhk/runeverything/internal/i18n"
	"github.com/foqerhk/runeverything/internal/identity"
	"github.com/getlantern/systray"
)

func cmdTray() {
	if !trayHasDesktopSession() {
		log.Println(i18n.T("tray.need_desktop"))
		os.Exit(2)
	}
	release, err := acquireAgentLock()
	if err != nil {
		log.Fatal(err)
	}
	defer release()

	// Graceful exit on SIGTERM so a replacement instance can take the lock.
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		systray.Quit()
	}()

	runtime.LockOSThread()
	systray.Run(onUnixTrayReady, func() {})
}

func onUnixTrayReady() {
	if runtime.GOOS == "darwin" {
		// Template icon: system tints for light/dark menu bar; no title text.
		systray.SetTemplateIcon(trayIconTemplatePNG, trayIconTemplatePNG)
	} else {
		systray.SetIcon(trayIconColorPNG)
	}
	systray.SetTitle("")
	systray.SetTooltip(i18n.T("tray.tooltip"))

	mQR := systray.AddMenuItem(i18n.T("tray.show_qr"), i18n.T("tray.show_qr_tip"))
	mCopy := systray.AddMenuItem(i18n.T("tray.copy_link"), i18n.T("tray.copy_link_tip"))
	mFolder := systray.AddMenuItem(i18n.T("tray.open_folder"), i18n.T("tray.open_folder_tip"))
	systray.AddSeparator()
	mPerm := systray.AddMenuItem(i18n.T("tray.check_perms"), i18n.T("tray.check_perms_tip"))
	mAuto := systray.AddMenuItemCheckbox(i18n.T("tray.autostart"), i18n.T("tray.autostart_tip"), trayAutostartEnabled())
	systray.AddSeparator()
	mQuit := systray.AddMenuItem(i18n.T("tray.quit"), i18n.T("tray.quit_tip"))

	if runtime.GOOS == "darwin" {
		logHostPermissions(desktop.EnsureHostPermissions())
	}

	var marquee trayMarquee
	agent, stopAwake, errCh := startTrayAgent(func(active bool) {
		marquee.SetActive(active)
	})
	if agent == nil {
		systray.SetTooltip(i18n.T("tray.tooltip_failed"))
		trayNotify(i18n.T("desktop.confirm_title"), i18n.T("tray.tooltip_failed"))
	} else {
		systray.SetTooltip(i18n.T("tray.tooltip_running"))
		// Pair offer happens inside runLoopRE2 after relay connect — do not call
		// offerPairRE2 here (re2Conn is still nil).
	}

	go func() {
		for {
			select {
			case <-mQR.ClickedCh:
				if agent == nil {
					trayNotify(i18n.T("desktop.confirm_title"), i18n.T("tray.not_running"))
					continue
				}
				path, link, err := showPairQR(agent)
				if err != nil {
					trayAlert(i18n.T("desktop.confirm_title"), i18n.T("tray.pair_failed", err.Error()))
					continue
				}
				_ = trayOpenPath(path)
				_ = traySetClipboard(link)
				trayNotify(i18n.T("desktop.confirm_title"), i18n.T("tray.qr_opened"))
			case <-mCopy.ClickedCh:
				if agent == nil {
					continue
				}
				_, link, err := showPairQR(agent)
				if err != nil {
					trayAlert(i18n.T("desktop.confirm_title"), err.Error())
					continue
				}
				_ = traySetClipboard(link)
				trayNotify(i18n.T("desktop.confirm_title"), i18n.T("tray.link_copied"))
			case <-mFolder.ClickedCh:
				home, _ := identity.HomeDir()
				_ = trayOpenFolder(home)
			case <-mPerm.ClickedCh:
				trayCheckPermissions()
			case <-mAuto.ClickedCh:
				exe, _ := os.Executable()
				if trayAutostartEnabled() {
					if err := trayDisableAutostart(); err != nil {
						trayAlert(i18n.T("desktop.confirm_title"), err.Error())
						continue
					}
					mAuto.Uncheck()
					trayNotify(i18n.T("desktop.confirm_title"), i18n.T("tray.autostart_off"))
				} else {
					if err := trayEnableAutostart(exe); err != nil {
						trayAlert(i18n.T("desktop.confirm_title"), err.Error())
						continue
					}
					mAuto.Check()
					trayNotify(i18n.T("desktop.confirm_title"), i18n.T("tray.autostart_on"))
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

func trayCheckPermissions() {
	if runtime.GOOS != "darwin" {
		trayNotify(i18n.T("desktop.confirm_title"), i18n.T("tray.perms_linux_hint"))
		return
	}
	p := desktop.EnsureHostPermissions()
	logHostPermissions(p)
	if p.ScreenRecording && p.Accessibility {
		trayAlert(i18n.T("desktop.confirm_title"), i18n.T("log.perm_ok"))
		return
	}
	msg := ""
	if !p.ScreenRecording {
		msg += i18n.T("log.perm_screen_need") + "\n\n"
	}
	if !p.Accessibility {
		msg += i18n.T("log.perm_ax_need") + "\n\n"
	}
	msg += i18n.T("log.perm_mic_hint")
	trayAlert(i18n.T("desktop.confirm_title"), msg)
}
