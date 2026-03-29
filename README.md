# fcm

The CLI for [Firecracker](https://github.com/firecracker-microvm/firecracker) microVMs. One command to launch a full Linux VM with SSH access.

Unlike containers, each VM runs its own Linux kernel with full isolation. Unlike QEMU/libvirt, boot time is seconds and memory overhead is minimal.

## Quick Start

```bash
# Requirements: Linux with /dev/kvm
# Install dependencies (one-time)
sudo apt-get install -y qemu-utils fdisk e2fsprogs mtools   # Debian/Ubuntu
# sudo dnf install -y qemu-img util-linux e2fsprogs mtools  # Fedora/RHEL

# Download fcm (or build from source: make docker-build && make build)
sudo cp bin/fcm /usr/local/bin/fcm

# One-time setup — downloads Firecracker + kernel, configures networking
sudo fcm init

# Create a VM (first run downloads ~700MB cloud image)
sudo fcm create myvm --image ubuntu-24.04 --ssh-key ~/.ssh/id_ed25519.pub
#  VM myvm created and started:
#    IP:     192.168.100.10
#    CPUs:   2
#    Memory: 1024 MB
#    Disk:   10 GB
#
#  Access:
#    SSH:     ssh root@192.168.100.10
#    Console: fcm console myvm

# SSH in
sudo fcm ssh myvm
```

## Supported Images

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

```
fcm init                          # One-time setup
fcm create <name> --image <img>   # Create and start a VM
fcm list                          # List all VMs
fcm ssh <name>                    # SSH into a VM
fcm console <name>                # Serial console (Ctrl+] to detach)
fcm stop <name>                   # Stop a VM
fcm start <name>                  # Start a stopped VM
fcm restart <name>                # Restart a VM
fcm delete <name> [--force]       # Delete a VM (--force if running)
fcm inspect <name>                # Show VM details (JSON)
fcm logs <name> [--follow]        # View systemd logs
fcm backup <name>                 # Backup VM disk
fcm restore <name> <backup>       # Restore from backup
fcm backups <name>                # List backups
fcm images                        # List local images
fcm pull <image>                  # Download a cloud image
fcm doctor                        # Check system readiness
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
# Use a custom cloud-init config
fcm create myvm --image ubuntu-24.04 --cloud-init ./my-config.yaml
```

Example templates in `env/`:
- `env/tailscale.yaml` — install and join a Tailscale network
- `env/docker.yaml` — install Docker CE
- `env/base.yaml` — minimal template

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

## What `fcm init` Does

Transparency matters when a tool runs as root. Here is exactly what `fcm init` does to your system:

1. Creates `/var/lib/fcm/` directory structure
2. Downloads the Firecracker binary to `/usr/local/bin/firecracker`
3. Downloads a Linux kernel to `/var/lib/fcm/kernels/vmlinux-default`
4. Writes `/var/lib/fcm/config.json` with default settings
5. Creates a network bridge `fcbr0` with IP `192.168.100.1/24`
6. Adds iptables NAT rules for VM internet access
7. Installs three systemd services:
   - `fcm-bridge.service` — manages the network bridge
   - `fcm-dhcp.service` — embedded DHCP server for VMs
   - `fcm-vm-<name>.service` — one per VM (created by `fcm create`)

## License

[MIT](LICENSE)
