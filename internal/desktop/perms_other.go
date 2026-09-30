//go:build !darwin

package desktop

// HostPermissions is a no-op summary on non-macOS hosts.
type HostPermissions struct {
	ScreenRecording bool
	Accessibility   bool
}

func CheckHostPermissions() HostPermissions {
	return HostPermissions{ScreenRecording: true, Accessibility: true}
}

func EnsureHostPermissions() HostPermissions { return CheckHostPermissions() }

func RequestScreenRecording() bool { return true }

func RequestAccessibility() bool { return true }

func OpenPrivacySettings(kind string) error { return nil }

func (p HostPermissions) Missing() []string { return nil }
