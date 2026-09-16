package directory

import "testing"

func TestSharingEnabled(t *testing.T) {
	t.Setenv("RE_SHARE_RELAY", "")
	off := false
	on := true
	if !SharingEnabled(nil) {
		t.Fatal("default on")
	}
	if SharingEnabled(&off) {
		t.Fatal("cfg off")
	}
	if !SharingEnabled(&on) {
		t.Fatal("cfg on")
	}
	t.Setenv("RE_SHARE_RELAY", "0")
	if SharingEnabled(&on) {
		t.Fatal("env should override to off")
	}
}
