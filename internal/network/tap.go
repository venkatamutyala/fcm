package network

import (
	"crypto/sha256"
	"fmt"
)

// CreateTAP creates a TAP device and attaches it to the bridge.
func CreateTAP(tapName, bridgeName string) error {
	// Remove stale TAP if it exists (idempotent)
	if linkExists(tapName) {
		if err := DeleteTAP(tapName); err != nil {
			return fmt.Errorf("remove stale tap %s: %w", tapName, err)
		}
	}

	if err := run("ip", "tuntap", "add", "dev", tapName, "mode", "tap"); err != nil {
		return fmt.Errorf("create tap: %w", err)
	}

	if err := run("ip", "link", "set", tapName, "up"); err != nil {
		_ = DeleteTAP(tapName) // cleanup on failure
		return fmt.Errorf("bring up tap: %w", err)
	}

	if err := run("ip", "link", "set", tapName, "master", bridgeName); err != nil {
		_ = DeleteTAP(tapName) // cleanup on failure
		return fmt.Errorf("attach tap to bridge: %w", err)
	}

	return nil
}

// DeleteTAP removes a TAP device. Always succeeds (idempotent).
func DeleteTAP(tapName string) error {
	if !linkExists(tapName) {
		return nil
	}
	return run("ip", "link", "del", tapName)
}

// TAPName generates a TAP device name for a VM.
// TAP device names are limited to 15 characters on Linux.
// Uses a hash suffix to avoid collisions for long VM names.
func TAPName(vmName string) string {
	const maxLen = 15
	prefix := "fc"
	// Use first few chars + hash of full name to avoid collisions
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(vmName)))[:6]
	name := prefix + hash
	if len(name) > maxLen {
		name = name[:maxLen]
	}
	return name
}
