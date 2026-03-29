package network

import (
	"testing"
)

func TestMACFromIP(t *testing.T) {
	tests := []struct {
		ip   string
		want string
	}{
		{"192.168.100.10", "AA:FC:00:00:64:0A"},
		{"192.168.100.254", "AA:FC:00:00:64:FE"},
		{"10.0.0.1", "AA:FC:00:00:00:01"},
		{"invalid", "AA:FC:00:00:00:01"},
	}

	for _, tt := range tests {
		got := MACFromIP(tt.ip)
		if got != tt.want {
			t.Errorf("MACFromIP(%q) = %q, want %q", tt.ip, got, tt.want)
		}
	}
}

func TestBootArgs(t *testing.T) {
	args := BootArgs("192.168.100.10", "192.168.100.1", "255.255.255.0")
	expected := "console=ttyS0 reboot=k panic=1 net.ifnames=0 biosdevname=0 ip=192.168.100.10::192.168.100.1:255.255.255.0::eth0:off"
	if args != expected {
		t.Errorf("BootArgs() = %q, want %q", args, expected)
	}
}

func TestTAPName(t *testing.T) {
	tests := []struct {
		vmName string
		want   string
	}{
		{"dev", "fcm-dev"},
		{"web1", "fcm-web1"},
		{"a-very-long-vm-name", "fcm-a-very-long"}, // truncated to 15 chars
		{"x", "fcm-x"},
	}

	for _, tt := range tests {
		got := TAPName(tt.vmName)
		if got != tt.want {
			t.Errorf("TAPName(%q) = %q, want %q", tt.vmName, got, tt.want)
		}
		if len(got) > 15 {
			t.Errorf("TAPName(%q) = %q, length %d exceeds 15", tt.vmName, got, len(got))
		}
	}
}
