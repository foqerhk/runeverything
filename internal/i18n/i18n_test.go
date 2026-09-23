package i18n

import "testing"

func TestNormalizeChineseVariants(t *testing.T) {
	for _, in := range []string{"zh", "zh_CN", "zh-TW", "zh_HK.UTF-8", "zh_Hant", "Chinese"} {
		if normalize(in) != Zh {
			t.Fatalf("%q want zh", in)
		}
	}
	for _, in := range []string{"en", "en_US", "C", "posix", "ja_JP", "fr_FR"} {
		if normalize(in) != En {
			t.Fatalf("%q want en", in)
		}
	}
}

func TestTZh(t *testing.T) {
	Set(Zh)
	if got := T("log.behind_nat"); got == en["log.behind_nat"] {
		t.Fatalf("expected chinese: %q", got)
	}
	Set(En)
	if got := T("log.behind_nat"); got != en["log.behind_nat"] {
		t.Fatalf("expected english: %q", got)
	}
}

func TestRELangOverride(t *testing.T) {
	t.Setenv("RE_LANG", "zh-TW")
	t.Setenv("LANG", "en_US.UTF-8")
	mu.Lock()
	inited = false
	mu.Unlock()
	Init()
	if Active() != Zh {
		t.Fatalf("RE_LANG zh-TW should map to zh, got %s", Active())
	}
}
