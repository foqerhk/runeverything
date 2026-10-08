//go:build !darwin

package desktop

// HostPermissions is a no-op summary on non-macOS hosts.
type HostPermissions struct {
	ScreenRecording bool
	Accessibility   bool
	Microphone      bool
	Camera          bool
}

// HostPermissionsEffective mirrors darwin; restart flags are always false here.
type HostPermissionsEffective struct {
	HostPermissions
	ScreenNeedsRestart bool
	AxNeedsRestart     bool
}

func CheckHostPermissions() HostPermissions {
	return HostPermissions{ScreenRecording: true, Accessibility: true, Microphone: true, Camera: true}
}

func CheckHostPermissionsEffective() HostPermissionsEffective {
	return HostPermissionsEffective{HostPermissions: CheckHostPermissions()}
}

func EnsureHostPermissions() HostPermissions { return CheckHostPermissions() }

func RequestScreenRecording() bool { return true }

func RequestAccessibility() bool { return true }

func RequestMicrophone() bool { return true }

func RequestCamera() bool { return true }

func MicrophoneCanPrompt() bool { return false }

func CameraCanPrompt() bool { return false }

func OpenPrivacySettings(kind string) error { return nil }

func (p HostPermissions) Missing() []string { return nil }
