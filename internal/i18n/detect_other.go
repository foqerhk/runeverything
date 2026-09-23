//go:build !windows

package i18n

func platformPrefersChinese() bool {
	return false
}
