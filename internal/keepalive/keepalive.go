package keepalive

import (
	"log"
	"os"
	"strings"
)

// Start prevents the machine from idling to sleep while the agent runs.
// Returns a stop function. No-op when RE_KEEP_AWAKE=0.
func Start() (stop func()) {
	if disabled() {
		return func() {}
	}
	stop, err := platformStart()
	if err != nil {
		log.Printf("keepalive: %v (machine may still sleep)", err)
		return func() {}
	}
	log.Printf("keepalive: preventing idle sleep (set RE_KEEP_AWAKE=0 to disable)")
	return stop
}

func disabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("RE_KEEP_AWAKE")))
	return v == "0" || v == "false" || v == "off" || v == "no"
}
