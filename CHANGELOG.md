# Changelog

All notable changes to fcm will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and this project adheres to [Semantic Versioning](https://semver.org/).

## [0.1.0] - 2026-03-29

### Added

- `fcm init` — one-time setup (downloads Firecracker + kernel, configures networking)
- `fcm create` / `fcm delete` — VM lifecycle
- `fcm run` — create + wait for SSH + connect in one command
- `fcm freeze` / `fcm unfreeze` — pause and resume VMs via Firecracker snapshots
- `fcm ssh` / `fcm exec` / `fcm console` — VM access (SSH, command execution, serial log)
- `fcm cp` — copy files to/from VMs via SCP
- `fcm stats` — VM resource usage (CPU, memory, disk, network)
- `fcm resize` — change CPU, memory, or disk after creation
- `fcm freeze` / `fcm unfreeze` — pause and resume VMs (snapshot-based, replaces backup/restore)
- `fcm templates` — built-in VM templates (ubuntu, ubuntu-dev, debian)
- `fcm images` / `fcm pull` — image management
- `fcm cleanup` — remove all VMs, services, and FCM state
- `fcm doctor` — system health check
- Embedded DHCP server (pure Go, no external dependencies)
- Cloud-init via CIDATA vfat disk
- Custom kernel from Amazon Linux source (vfat, iso9660, btrfs, containers, TUN)
- Configurable subnet (`fcm init --subnet`)
- Static IP assignment (`fcm create --ip`)
- Auto-detect SSH key from ~/.ssh/*.pub
- Download progress bars with speed
- Boot timing display ("VM ready! booted in 3.2s")
- Tab completion for VM names, images, and templates
- Structured error messages with fix suggestions
- Auto-install host dependencies in `fcm init`
- Signed releases with cosign (keyless)
- Docker container on ghcr.io/venkatamutyala/fcm

### Supported Images

- Ubuntu 24.04, Ubuntu 22.04
- Debian 12
- Rocky Linux 9, AlmaLinux 9, CentOS Stream 9
- openSUSE Leap 15.6
