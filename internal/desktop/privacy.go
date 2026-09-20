package desktop

import (
	"fmt"
	"os"
	"sync"
)

// Privacy blank-screen: covers local displays while remote session is active.
var (
	privacyMu    sync.Mutex
	privacyOn    bool
	privacyStop  func()
)

func SetPrivacyBlank(on bool) error {
	privacyMu.Lock()
	defer privacyMu.Unlock()
	if on == privacyOn {
		return nil
	}
	if on {
		stop, err := startPrivacyBlank()
		if err != nil {
			return err
		}
		privacyStop = stop
		privacyOn = true
		return nil
	}
	if privacyStop != nil {
		privacyStop()
		privacyStop = nil
	}
	privacyOn = false
	return nil
}

func PrivacyBlankEnabled() bool {
	privacyMu.Lock()
	defer privacyMu.Unlock()
	return privacyOn
}

// startPrivacyBlank is implemented per OS; stub returns unsupported.
func startPrivacyBlank() (func(), error) {
	if os.Getenv("RE_PRIVACY") == "0" {
		return func() {}, nil
	}
	return startPrivacyBlankOS()
}

func privacyUnsupported() (func(), error) {
	return nil, fmt.Errorf("privacy blank not supported on this OS")
}
