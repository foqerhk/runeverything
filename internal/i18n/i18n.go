package i18n

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

// Lang is the UI/log language. Only "zh" (Simplified) and "en" are used.
type Lang string

const (
	En Lang = "en"
	Zh Lang = "zh" // Simplified Chinese; also used for zh-TW / zh-HK / zh-Hant
)

var (
	mu      sync.RWMutex
	current = En
	inited  bool
)

// Init detects language once. Safe to call multiple times.
// Override with RE_LANG=zh|en|zh-CN|zh-TW|… (Traditional locales still map to Simplified).
func Init() {
	mu.Lock()
	defer mu.Unlock()
	if inited {
		return
	}
	current = detectLocked()
	inited = true
}

// Set forces a language (tests / rare overrides). Marks Init as done.
func Set(lang Lang) {
	mu.Lock()
	defer mu.Unlock()
	if lang != Zh {
		lang = En
	}
	current = lang
	inited = true
}

// Active returns the current language.
func Active() Lang {
	Init()
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// IsZh reports Simplified Chinese UI/logs.
func IsZh() bool { return Active() == Zh }

func detectLocked() Lang {
	if v := strings.TrimSpace(os.Getenv("RE_LANG")); v != "" {
		return normalize(v)
	}
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return normalize(v)
		}
	}
	if platformPrefersChinese() {
		return Zh
	}
	return En
}

func normalize(raw string) Lang {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, "-", "_")
	if i := strings.IndexAny(s, ".@"); i >= 0 {
		s = s[:i]
	}
	switch s {
	case "zh", "zh_cn", "zh_sg", "zh_hans", "zh_tw", "zh_hk", "zh_mo", "zh_hant",
		"chinese", "chinese_simplified", "chinese_traditional":
		return Zh
	case "en", "en_us", "en_gb", "c", "posix":
		return En
	}
	if strings.HasPrefix(s, "zh") {
		return Zh
	}
	return En
}

// T looks up a message key and formats with fmt.Sprintf when args are given.
func T(key string, args ...any) string {
	Init()
	mu.RLock()
	lang := current
	mu.RUnlock()
	msg := lookup(lang, key)
	if len(args) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, args...)
}

// Log prints a localized message via the standard logger (avoids vet format warnings).
func Log(key string, args ...any) {
	logPrint(T(key, args...))
}

// logPrint is assigned to log.Print so tests can swap if needed.
var logPrint = func(v ...any) { log.Print(v...) }

func lookup(lang Lang, key string) string {
	if lang == Zh {
		if s, ok := zh[key]; ok {
			return s
		}
	}
	if s, ok := en[key]; ok {
		return s
	}
	return key
}
