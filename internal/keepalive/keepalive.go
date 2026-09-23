package keepalive

import (
	"os"
	"strings"

	"github.com/foqerhk/runeverything/internal/i18n"
)

// Start prevents the machine from idling to sleep while the agent runs.
// Returns a stop function. No-op when RE_KEEP_AWAKE=0.
func Start() (stop func()) {
	if disabled() {
		return func() {}
	}
	stop, err := platformStart()
	if err != nil {
		i18n.Log("log.keepalive_fail", err)
		return func() {}
	}
	i18n.Log("log.keepalive")
	return stop
}

func disabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("RE_KEEP_AWAKE")))
	return v == "0" || v == "false" || v == "off" || v == "no"
}
