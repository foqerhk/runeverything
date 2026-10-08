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
		trayApplyAppIcon()
	} else {
		systray.SetIcon(trayIconColorPNG)
	}
	systray.SetTitle("")
	systray.SetTooltip(i18n.T("tray.tooltip"))

	mQR := systray.AddMenuItem(i18n.T("tray.show_qr"), i18n.T("tray.show_qr_tip"))
	mCopy := systray.AddMenuItem(i18n.T("tray.copy_link"), i18n.T("tray.copy_link_tip"))
	mFolder := systray.AddMenuItem(i18n.T("tray.open_folder"), i18n.T("tray.open_folder_tip"))
	systray.AddSeparator()
	mStatus := systray.AddMenuItem(i18n.T("tray.status"), i18n.T("tray.status_tip"))
	mConfig := systray.AddMenuItem(i18n.T("tray.config"), i18n.T("tray.config_tip"))
	mPerm := systray.AddMenuItem(i18n.T("tray.check_perms"), i18n.T("tray.check_perms_tip"))
	mHelp := systray.AddMenuItem(i18n.T("tray.help"), i18n.T("tray.help_tip"))
	mAbout := systray.AddMenuItem(i18n.T("tray.about"), i18n.T("tray.about_tip"))
	systray.AddSeparator()
	mAuto := systray.AddMenuItemCheckbox(i18n.T("tray.autostart"), i18n.T("tray.autostart_tip"), trayAutostartEnabled())
	systray.AddSeparator()
	mQuit := systray.AddMenuItem(i18n.T("tray.quit"), i18n.T("tray.quit_tip"))

	if runtime.GOOS == "darwin" {
		logHostPermissions(desktop.CheckHostPermissions())
		trayMaybeShowPermissionsOnFirstRun()
		if err := desktop.StartVirtualFromEnv(); err != nil {
			log.Printf("vdisplay: %v", err)
		}
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
	}

	go func() {
		for {
			select {
			case <-mQR.ClickedCh:
				go func() {
					if agent == nil {
						trayNotify(i18n.T("desktop.confirm_title"), i18n.T("tray.not_running"))
						return
					}
					if runtime.GOOS == "darwin" {
						if err := trayShowQRWindow(); err != nil {
							trayAlert(i18n.T("desktop.confirm_title"), i18n.T("tray.pair_failed", err.Error()))
							return
						}
						trayNotify(i18n.T("desktop.confirm_title"), i18n.T("tray.qr_opened"))
						return
					}
					path, link, err := showPairQR(agent)
					if err != nil {
						trayAlert(i18n.T("desktop.confirm_title"), i18n.T("tray.pair_failed", err.Error()))
						return
					}
					_ = trayOpenPath(path)
					_ = traySetClipboard(link)
					trayNotify(i18n.T("desktop.confirm_title"), i18n.T("tray.qr_opened"))
				}()
			case <-mCopy.ClickedCh:
				go func() {
					if agent == nil {
						return
					}
					_, link, err := showPairQR(agent)
					if err != nil {
						trayAlert(i18n.T("desktop.confirm_title"), err.Error())
						return
					}
					_ = traySetClipboard(link)
					trayNotify(i18n.T("desktop.confirm_title"), i18n.T("tray.link_copied"))
				}()
			case <-mFolder.ClickedCh:
				go func() {
					home, _ := identity.HomeDir()
					_ = trayOpenFolder(home)
				}()
			case <-mStatus.ClickedCh:
				go trayShowStatusWindow()
			case <-mConfig.ClickedCh:
				go trayShowConfigWindow()
			case <-mPerm.ClickedCh:
				go trayShowPermissionsPanel()
			case <-mHelp.ClickedCh:
				go trayShowHelpWindow()
			case <-mAbout.ClickedCh:
				go trayShowAboutWindow()
			case <-mAuto.ClickedCh:
				go func() {
					exe, _ := os.Executable()
					if trayAutostartEnabled() {
						if err := trayDisableAutostart(); err != nil {
							trayAlert(i18n.T("desktop.confirm_title"), err.Error())
							return
						}
						mAuto.Uncheck()
						trayNotify(i18n.T("desktop.confirm_title"), i18n.T("tray.autostart_off"))
					} else {
						if err := trayEnableAutostart(exe); err != nil {
							trayAlert(i18n.T("desktop.confirm_title"), err.Error())
							return
						}
						mAuto.Check()
						trayNotify(i18n.T("desktop.confirm_title"), i18n.T("tray.autostart_on"))
					}
				}()
			case <-mQuit.ClickedCh:
				_ = desktop.DestroyAllVirtual()
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
