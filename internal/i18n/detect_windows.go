//go:build windows

package i18n

import "golang.org/x/sys/windows"

// LANG_CHINESE primary language ID (0x04). Covers zh-CN / zh-TW / zh-HK UI.
const langChinese = 0x04

var (
	modKernel32              = windows.NewLazySystemDLL("kernel32.dll")
	procGetUserDefaultUILang = modKernel32.NewProc("GetUserDefaultUILanguage")
)

func platformPrefersChinese() bool {
	r, _, _ := procGetUserDefaultUILang.Call()
	if r == 0 {
		return false
	}
	return int(r)&0x3ff == langChinese
}
