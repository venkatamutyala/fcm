# fcm

The CLI for [Firecracker](https://github.com/firecracker-microvm/firecracker) microVMs. One command to launch a full Linux VM with SSH access.

<!-- TODO: Replace with actual asciinema recording
[![asciicast](https://asciinema.org/a/XXXXX.svg)](https://asciinema.org/a/XXXXX)
-->

```bash
$ sudo fcm run myvm --image ubuntu-24.04
  Downloading ubuntu-24.04... 342/680 MB  12.3 MB/s
  Preparing rootfs (10 GB)...
  Generating cloud-init...
  Starting myvm...
  Waiting for VM to boot... (3.2s)
  VM ready! (booted in 3.2s)

root@myvm:~# uname -a
Linux myvm 6.1.102 #1 SMP x86_64 GNU/Linux
```

Unlike containers, each VM runs its own Linux kernel with full isolation. Unlike QEMU/libvirt, boot time is seconds and memory overhead is minimal.

## Install

```bash
# One-line install (Linux x86_64 or arm64, requires /dev/kvm)
curl -fsSL https://raw.githubusercontent.com/venkatamutyala/fcm/main/install.sh | sudo bash

# One-time setup — downloads Firecracker + kernel, configures networking
sudo fcm init
```

That's it. Now create a VM:

> **Note:** VMs have a default root password `fcm` for console access. SSH key authentication is the primary access method.

```bash
# Launch a VM and SSH in (auto-detects your SSH key)
sudo fcm run myvm --image ubuntu-24.04
```

Or step by step:

```bash
sudo fcm create myvm --image ubuntu-24.04    # Create VM
sudo fcm ssh myvm                            # SSH in
sudo fcm freeze myvm                         # Freeze (pause + save state)
sudo fcm unfreeze myvm                       # Unfreeze (resume from state)
sudo fcm delete myvm --force                 # Delete
```

## Why fcm?

| | fcm | Docker | Vagrant | multipass | LXD/Incus |
|---|---|---|---|---|---|
| Real kernel isolation | Yes | No | Yes | Yes | Partial |
| Boot time | ~3s | ~0s | ~30s | ~15s | ~5s |
| Memory overhead | ~50MB | ~0 | ~512MB | ~512MB | ~100MB |
| Single binary | Yes | No | No | No | No |
| No daemon required | Yes | No | Yes | No | No |
| Multi-distro images | Yes | Yes | Yes | Ubuntu only | Yes |
| Runs on bare metal | Yes | Yes | Yes | Yes | Yes |
| Cloud-init support | Yes | No | Yes | Yes | Yes |

## Supported Images

```bash
sudo fcm images --available    # List all pullable images
```

| Image | Filesystem | Tested |
|-------|-----------|--------|
| `ubuntu-24.04` | ext4 | Yes |
| `ubuntu-22.04` | ext4 | Yes |
| `debian-12` | ext4 | Yes |
| `rocky-9` | XFS | Yes |
| `alma-9` | XFS | Yes |
| `centos-stream9` | XFS | Yes |
| `opensuse-15.6` | XFS | Yes |

## Commands

```bash
# The basics
fcm run <name> --image <img>      # Create + wait for SSH + connect (one command)
fcm create <name> --image <img>   # Create and start a VM
fcm list                          # List all VMs
fcm ssh <name>                    # SSH into a VM
fcm exec <name> -- <cmd>          # Run a command in a VM
fcm console <name>                # Serial console (Ctrl+] to detach)

# Lifecycle
fcm freeze <name>                 # Freeze a VM (pause + save state)
fcm unfreeze <name>               # Unfreeze a VM (resume from state)
fcm delete <name> [--force]       # Delete a VM
fcm resize <name> --cpus 4        # Resize CPU/memory/disk

# Images
fcm images                        # List local images
fcm images --available            # List all pullable images
fcm pull <image>                  # Download a cloud image

# Backups
fcm backup <name>                 # Backup VM disk
fcm restore <name> <backup>       # Restore from backup

# System
fcm init                          # One-time setup
fcm doctor                        # System health check
fcm cleanup --confirm             # Remove all VMs and FCM state
fcm inspect <name>                # VM details (JSON)
fcm logs <name> [--follow]        # systemd logs
fcm version                       # Print version
```

## How It Works

fcm creates an isolated virtual network on your host and manages Firecracker VM lifecycles through systemd:

```
Host Machine (any Linux with /dev/kvm)
│
├── fcm binary ─── embedded DHCP server (pure Go)
├── firecracker    (downloaded by fcm init)
├── vmlinux kernel (custom build with vfat/cloud-init support)
│
├── fcbr0 bridge ─── 192.168.100.0/24
│   ├── NAT to internet (iptables)
│   ├── tap0 → VM 1 (192.168.100.10)
│   ├── tap1 → VM 2 (192.168.100.11)
│   └── ...
│
└── /var/lib/fcm/
    ├── config.json
    ├── kernels/vmlinux-default
    ├── images/          (pulled cloud images)
    └── vms/<name>/
        ├── vm.json      (VM config)
        ├── rootfs.ext4  (root filesystem)
        ├── cidata.img   (cloud-init data)
        └── console.log  (serial output)
```

Each VM is a systemd service (`fcm-vm-<name>.service`) with automatic restart on failure. Networking is handled by an embedded DHCP server — no external dnsmasq or network configuration needed.

Cloud-init configures SSH keys, hostname, and networking via a CIDATA vfat disk attached to each VM. This works across all distros without any distro-specific patches.

## Custom Cloud-Init

```bash
fcm create myvm --image ubuntu-24.04 --cloud-init ./my-config.yaml
```

Example templates in `env/`:
- `env/tailscale.yaml` — install and join a Tailscale network
- `env/docker.yaml` — install Docker CE
- `env/base.yaml` — minimal template

## What `fcm init` Does

Transparency matters when a tool runs as root. Here is exactly what `fcm init` does to your system:

1. Checks and installs required host packages (`qemu-utils`, `mtools`, etc.)
2. Creates `/var/lib/fcm/` directory structure
3. Downloads the Firecracker binary to `/usr/local/bin/firecracker`
4. Downloads a Linux kernel to `/var/lib/fcm/kernels/vmlinux-default`
5. Writes `/var/lib/fcm/config.json` with default settings
6. Creates a network bridge `fcbr0` with IP `192.168.100.1/24`
7. Adds iptables NAT rules for VM internet access
8. Installs and starts systemd services:
   - `fcm-bridge.service` — manages the network bridge
   - `fcm-dhcp.service` — embedded DHCP server for VMs

To fully undo: `sudo fcm cleanup --confirm`

## Development

```bash
# Build (requires Docker)
make docker-build   # Build dev image (once)
make build          # Compile to ./bin/fcm
make test           # Run tests

# Build the custom kernel (only needed once)
docker build -t fcm-kernel -f kernel/Dockerfile kernel/

# Cross-compile
make release        # Builds linux-amd64 and linux-arm64
```

## License

[MIT](LICENSE)
