package registrar

import "testing"

func TestValidPublicIP(t *testing.T) {
	if ValidPublicIP("127.0.0.1") || ValidPublicIP("10.0.0.1") || ValidPublicIP("192.168.1.1") {
		t.Fatal("private/loopback should be rejected")
	}
	if !ValidPublicIP("1.1.1.1") {
		t.Fatal("public IP should be accepted")
	}
}
