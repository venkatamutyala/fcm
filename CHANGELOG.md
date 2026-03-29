# Changelog

All notable changes to fcm will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and this project adheres to [Semantic Versioning](https://semver.org/).

## [0.1.0] - Unreleased

### Added

- `fcm init` - Initialize fcm: download Firecracker, kernel, and rootfs images
- `fcm create` - Create a new Firecracker microVM with cloud-init support
- `fcm start` - Start a stopped VM
- `fcm stop` - Stop a running VM
- `fcm delete` - Delete a VM and clean up all resources
- `fcm list` - List all VMs and their status
- `fcm ssh` - SSH into a running VM
- `fcm console` - Attach to a VM serial console
- `fcm inspect` - Show detailed VM configuration and status
- `fcm logs` - View VM logs
- `fcm backup` - Backup VM configuration and data
- `fcm update` - Self-update fcm to the latest version
- `fcm doctor` - Diagnose system readiness (KVM, networking, dependencies)
- `fcm images` - Manage kernel and rootfs images
- `fcm configure` - Configure VM settings
- Automatic tap device and iptables NAT networking setup
- systemd service integration for VM lifecycle
- Cloud-init support for VM provisioning
- Custom kernel build support
- Docker-based development workflow
