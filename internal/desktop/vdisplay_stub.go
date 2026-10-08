//go:build !darwin

package desktop

import "fmt"

// VDisplayAvailable is always false off Darwin.
func VDisplayAvailable() bool { return false }

// EnsureVirtual is unsupported off Darwin.
func EnsureVirtual(mode string) (cgID uint32, w, h int, err error) {
	return 0, 0, 0, fmt.Errorf("virtual display unsupported on this platform")
}

// DestroyVirtual is a no-op off Darwin.
func DestroyVirtual(cgID uint32) error { return nil }

// DestroyAllVirtual is a no-op off Darwin.
func DestroyAllVirtual() error { return nil }

// VirtualDisplayIDs returns nil off Darwin.
func VirtualDisplayIDs() map[uint32]string { return nil }

// VirtualFramebuffer is unsupported off Darwin.
func VirtualFramebuffer(id int) (fbW, fbH int, ok bool) { return 0, 0, false }

// IsVirtualDisplay is always false off Darwin.
func IsVirtualDisplay(id int) bool { return false }

// StartVirtualFromEnv is a no-op off Darwin.
func StartVirtualFromEnv() error { return nil }
