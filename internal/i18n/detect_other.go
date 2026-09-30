//go:build !windows && !darwin

package i18n

func platformPrefersChinese() bool {
	return false
}
