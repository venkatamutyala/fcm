package systemd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"fcm.dev/fcm-cli/internal/config"
	vmstate "fcm.dev/fcm-cli/internal/vm"
)

const unitDir = "/etc/systemd/system"

var vmUnitTemplate = template.Must(template.New("vm").Parse(`[Unit]
Description=FCM: {{.Name}} (Firecracker microVM)
After=network.target fcm-bridge.service fcm-dhcp.service
Requires=fcm-bridge.service fcm-dhcp.service

[Service]
Type=simple
ExecStartPre=/usr/local/bin/fcm _setup-vm {{.Name}}
ExecStart=/usr/local/bin/firecracker --api-sock {{.SocketPath}}
ExecStartPost=/usr/local/bin/fcm _configure-vm {{.Name}}
ExecStopPost=/usr/local/bin/fcm _cleanup-vm {{.Name}}
StandardOutput=file:{{.SerialLog}}
StandardError=file:{{.SerialLog}}
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
`))

var bridgeUnitTemplate = template.Must(template.New("bridge").Parse(`[Unit]
Description=FCM: Network Bridge
After=network.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/local/bin/fcm _setup-bridge
ExecStop=/usr/local/bin/fcm _teardown-bridge

[Install]
WantedBy=multi-user.target
`))

var dhcpUnitTemplate = template.Must(template.New("dhcp").Parse(`[Unit]
Description=FCM: DHCP Server
After=fcm-bridge.service
Requires=fcm-bridge.service

[Service]
Type=simple
ExecStart=/usr/local/bin/fcm _dhcp
Restart=on-failure
RestartSec=2

[Install]
WantedBy=multi-user.target
`))

var backupTimerTemplate = template.Must(template.New("backup-timer").Parse(`[Unit]
Description=FCM: {{.Interval}} backup for {{.Name}}

[Timer]
OnCalendar={{.Calendar}}
Persistent=true

[Install]
WantedBy=timers.target
`))

var backupServiceTemplate = template.Must(template.New("backup-service").Parse(`[Unit]
Description=FCM: backup {{.Name}}

[Service]
Type=oneshot
ExecStart=/usr/local/bin/fcm backup {{.Name}} --prune {{.Keep}}
`))

// VMUnitName returns the systemd unit name for a VM.
func VMUnitName(vmName string) string {
	return fmt.Sprintf("fcm-vm-%s.service", vmName)
}

// VMUnitPath returns the full path to a VM's systemd unit file.
func VMUnitPath(vmName string) string {
	return filepath.Join(unitDir, VMUnitName(vmName))
}

// WriteVMUnit generates and writes a systemd unit file for a VM.
func WriteVMUnit(v *vmstate.VM) error {
	var buf strings.Builder
	if err := vmUnitTemplate.Execute(&buf, v); err != nil {
		return fmt.Errorf("render vm unit template: %w", err)
	}

	if err := os.WriteFile(VMUnitPath(v.Name), []byte(buf.String()), 0644); err != nil {
		return fmt.Errorf("write vm unit: %w", err)
	}

	return DaemonReload()
}

// RemoveVMUnit removes a VM's systemd unit file.
func RemoveVMUnit(vmName string) error {
	path := VMUnitPath(vmName)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	// Disable first (ignore errors)
	_ = exec.Command("systemctl", "disable", VMUnitName(vmName)).Run()

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove vm unit: %w", err)
	}

	return DaemonReload()
}

// WriteBridgeUnit generates and writes the bridge systemd unit file.
func WriteBridgeUnit() error {
	var buf strings.Builder
	if err := bridgeUnitTemplate.Execute(&buf, nil); err != nil {
		return fmt.Errorf("render bridge unit template: %w", err)
	}

	path := filepath.Join(unitDir, "fcm-bridge.service")
	if err := os.WriteFile(path, []byte(buf.String()), 0644); err != nil {
		return fmt.Errorf("write bridge unit: %w", err)
	}

	return DaemonReload()
}

// WriteDHCPUnit generates and writes the DHCP server systemd unit file.
func WriteDHCPUnit() error {
	var buf strings.Builder
	if err := dhcpUnitTemplate.Execute(&buf, nil); err != nil {
		return fmt.Errorf("render dhcp unit template: %w", err)
	}

	path := filepath.Join(unitDir, "fcm-dhcp.service")
	if err := os.WriteFile(path, []byte(buf.String()), 0644); err != nil {
		return fmt.Errorf("write dhcp unit: %w", err)
	}

	return DaemonReload()
}

// Disable disables a systemd unit.
func Disable(unit string) error {
	return runSystemctl("disable", unit)
}

// Enable enables a systemd unit.
func Enable(unit string) error {
	return runSystemctl("enable", unit)
}

// Start starts a systemd unit.
func Start(unit string) error {
	return runSystemctl("start", unit)
}

// Stop stops a systemd unit.
func Stop(unit string) error {
	return runSystemctl("stop", unit)
}

// IsActive returns true if a systemd unit is active (running).
func IsActive(unit string) bool {
	return exec.Command("systemctl", "is-active", "--quiet", unit).Run() == nil
}

// DaemonReload reloads systemd's configuration.
func DaemonReload() error {
	return runSystemctl("daemon-reload")
}

func runSystemctl(args ...string) error {
	cmd := exec.Command("systemctl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return nil
}

// VMStatus returns the status of a VM's systemd unit.
func VMStatus(vmName string) string {
	unit := VMUnitName(vmName)
	if IsActive(unit) {
		return "running"
	}

	// Check if the unit exists
	if _, err := os.Stat(VMUnitPath(vmName)); os.IsNotExist(err) {
		return "unknown"
	}

	return "stopped"
}

// BackupTimerName returns the systemd timer name for VM backups.
func BackupTimerName(vmName string) string {
	return fmt.Sprintf("fcm-backup-%s.timer", vmName)
}

// WriteBackupTimer generates backup timer and service units.
func WriteBackupTimer(vmName string, interval string, keep int) error {
	calendar := intervalToCalendar(interval)

	data := struct {
		Name     string
		Interval string
		Calendar string
		Keep     int
	}{vmName, interval, calendar, keep}

	// Write timer
	var timerBuf strings.Builder
	if err := backupTimerTemplate.Execute(&timerBuf, data); err != nil {
		return fmt.Errorf("render backup timer: %w", err)
	}
	timerPath := filepath.Join(unitDir, BackupTimerName(vmName))
	if err := os.WriteFile(timerPath, []byte(timerBuf.String()), 0644); err != nil {
		return fmt.Errorf("write backup timer: %w", err)
	}

	// Write service
	var svcBuf strings.Builder
	if err := backupServiceTemplate.Execute(&svcBuf, data); err != nil {
		return fmt.Errorf("render backup service: %w", err)
	}
	svcPath := filepath.Join(unitDir, fmt.Sprintf("fcm-backup-%s.service", vmName))
	if err := os.WriteFile(svcPath, []byte(svcBuf.String()), 0644); err != nil {
		return fmt.Errorf("write backup service: %w", err)
	}

	if err := DaemonReload(); err != nil {
		return err
	}

	return Enable(BackupTimerName(vmName))
}

func intervalToCalendar(interval string) string {
	switch interval {
	case "hourly":
		return "*-*-* *:00:00"
	case "daily":
		return "*-*-* 03:00:00"
	case "weekly":
		return "Mon *-*-* 03:00:00"
	default:
		return interval // allow raw OnCalendar syntax
	}
}

// GetConfig is a convenience to get config without circular import.
func GetConfig() (*config.Config, error) {
	return config.Load()
}
