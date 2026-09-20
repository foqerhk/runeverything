//go:build !windows && !darwin && !linux

package desktop

func startPrivacyBlankOS() (func(), error) { return privacyUnsupported() }
