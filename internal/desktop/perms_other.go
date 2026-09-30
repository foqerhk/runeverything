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

func (p HostPermissions) Missing() []string { return nil }
