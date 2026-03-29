package systemd

import (
	"strings"
	"testing"

	vmstate "fcm.dev/fcm-cli/internal/vm"
)

func TestVMUnitName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"dev-box", "fcm-vm-dev-box.service"},
		{"web1", "fcm-vm-web1.service"},
	}

	for _, tt := range tests {
		got := VMUnitName(tt.name)
		if got != tt.want {
			t.Errorf("VMUnitName(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestVMUnitTemplate(t *testing.T) {
	v := &vmstate.VM{
		Name:       "test-vm",
		SocketPath: "/var/lib/fcm/vms/test-vm/fc.socket",
		SerialLog:  "/var/lib/fcm/vms/test-vm/console.log",
	}

	var buf strings.Builder
	if err := vmUnitTemplate.Execute(&buf, v); err != nil {
		t.Fatalf("execute template: %v", err)
	}

	unit := buf.String()

	// Check key properties
	checks := []string{
		"Description=FCM: test-vm (Firecracker microVM)",
		"Requires=fcm-bridge.service",
		"ExecStartPre=/usr/local/bin/fcm _setup-vm test-vm",
		"ExecStart=/usr/local/bin/firecracker --api-sock /var/lib/fcm/vms/test-vm/fc.socket",
		"ExecStartPost=/usr/local/bin/fcm _configure-vm test-vm",
		"ExecStopPost=/usr/local/bin/fcm _cleanup-vm test-vm",
		"StandardOutput=file:/var/lib/fcm/vms/test-vm/console.log",
		"Restart=on-failure",
	}

	for _, check := range checks {
		if !strings.Contains(unit, check) {
			t.Errorf("unit missing %q\nGot:\n%s", check, unit)
		}
	}
}

func TestBridgeUnitTemplate(t *testing.T) {
	var buf strings.Builder
	if err := bridgeUnitTemplate.Execute(&buf, nil); err != nil {
		t.Fatalf("execute template: %v", err)
	}

	unit := buf.String()

	checks := []string{
		"ExecStart=/usr/local/bin/fcm _setup-bridge",
		"ExecStop=/usr/local/bin/fcm _teardown-bridge",
		"RemainAfterExit=yes",
		"Type=oneshot",
	}

	for _, check := range checks {
		if !strings.Contains(unit, check) {
			t.Errorf("bridge unit missing %q\nGot:\n%s", check, unit)
		}
	}
}

func TestBackupTimerName(t *testing.T) {
	got := BackupTimerName("dev-box")
	want := "fcm-backup-dev-box.timer"
	if got != want {
		t.Errorf("BackupTimerName = %q, want %q", got, want)
	}
}

func TestIntervalToCalendar(t *testing.T) {
	tests := []struct {
		interval string
		want     string
	}{
		{"daily", "*-*-* 03:00:00"},
		{"hourly", "*-*-* *:00:00"},
		{"weekly", "Mon *-*-* 03:00:00"},
		{"*-*-* 05:00:00", "*-*-* 05:00:00"}, // raw passthrough
	}

	for _, tt := range tests {
		got := intervalToCalendar(tt.interval)
		if got != tt.want {
			t.Errorf("intervalToCalendar(%q) = %q, want %q", tt.interval, got, tt.want)
		}
	}
}
