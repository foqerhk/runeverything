//go:build darwin

package i18n

import (
	"os/exec"
	"strings"
)

// platformPrefersChinese reads the macOS preferred language / locale.
// GUI processes (menu-bar tray, LaunchAgent) often have empty LANG and only
// expose Chinese via AppleLanguages / AppleLocale.
func platformPrefersChinese() bool {
	if out, err := exec.Command("defaults", "read", "-g", "AppleLocale").Output(); err == nil {
		if normalize(strings.TrimSpace(string(out))) == Zh {
			return true
		}
	}
	out, err := exec.Command("defaults", "read", "-g", "AppleLanguages").Output()
	if err != nil {
		return false
	}
	s := strings.ToLower(string(out))
	// Typical: ( "zh-Hans-CN", "en-CN", … )
	for _, marker := range []string{"zh-hans", "zh_hans", "zh-cn", "zh_cn", `"zh"`, "zh-hant", "zh-tw", "zh-hk"} {
		if strings.Contains(s, marker) {
			// Prefer Chinese if it appears before English as the first quoted tag.
			return firstPreferredLanguageIsChinese(s)
		}
	}
	return false
}

func firstPreferredLanguageIsChinese(appleLanguagesDump string) bool {
	// Scan quoted language tags in order.
	rest := appleLanguagesDump
	for {
		i := strings.Index(rest, "\"")
		if i < 0 {
			return false
		}
		rest = rest[i+1:]
		j := strings.Index(rest, "\"")
		if j < 0 {
			return false
		}
		tag := rest[:j]
		rest = rest[j+1:]
		tag = strings.ToLower(strings.ReplaceAll(tag, "-", "_"))
		if tag == "" {
			continue
		}
		if strings.HasPrefix(tag, "zh") {
			return true
		}
		// First real language tag is not Chinese.
		return false
	}
}
