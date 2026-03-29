# fcm — The CLI for Firecracker

Lightweight CLI for running Firecracker microVMs on Linux. One binary, one command to create a full Linux VM with SSH access.

## Quick Start

```bash
# Install
sudo cp fcm /usr/local/bin/fcm

# One-time setup (downloads Firecracker, kernel, sets up networking)
sudo fcm init

# Create a VM
sudo fcm create myvm --image ubuntu-24.04 --ssh-key ~/.ssh/id_ed25519.pub

# SSH in
ssh root@192.168.100.10

# Or use the built-in shortcut
sudo fcm ssh myvm
```

## Features

- **Single binary** — no runtime dependencies, no Docker required
- **Multi-distro** — Ubuntu, Debian, Rocky, AlmaLinux, CentOS, openSUSE
- **Cloud-init** — SSH keys, networking, hostname configured automatically
- **Embedded DHCP** — self-contained networking, no external DHCP server
- **Fast** — VMs boot in seconds on Firecracker microVMs
- **Simple lifecycle** — create, start, stop, delete, list, inspect
- **Disk backups** — backup and restore VM root filesystems
- **Serial console** — interactive console access without networking

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
fcm delete <name> [--force]       # Delete a VM
fcm inspect <name>                # Show VM details (JSON)
fcm logs <name> [--follow]        # View systemd logs
fcm backup <name>                 # Backup VM disk
fcm restore <name> <backup>       # Restore from backup
fcm backups <name>                # List backups
fcm images                        # List available images
fcm pull <image>                  # Download an image
fcm doctor                        # Check system readiness
fcm version                       # Print version
```

## Supported Images

| Image | Filesystem | Status |
|-------|-----------|--------|
| `ubuntu-24.04` | ext4 | Tested |
| `ubuntu-22.04` | ext4 | Tested |
| `debian-12` | ext4 | Tested |
| `rocky-9` | XFS | Tested |
| `alma-9` | XFS | Tested |
| `centos-stream9` | XFS | Tested |
| `opensuse-15.6` | XFS | Tested |

## Requirements

- Linux with `/dev/kvm` (any distro)
- `qemu-img`, `sfdisk` for image conversion (pull-time only)
- `mtools` for cloud-init disk generation
- `e2fsprogs` for ext4 images

`fcm init` downloads Firecracker and the kernel automatically.

## Architecture

```
Host Machine
├── fcm binary (Go, single static binary)
├── firecracker (downloaded by fcm init)
├── vmlinux kernel (custom, built from Amazon Linux source with vfat support)
│
├── fcbr0 bridge (192.168.100.0/24)
│   ├── Embedded DHCP server (pure Go)
│   ├── NAT to internet (iptables MASQUERADE)
│   └── TAP devices per VM
│
└── /var/lib/fcm/
    ├── config.json
    ├── kernels/vmlinux-default
    ├── images/*.ext4        (pulled cloud images)
    └── vms/<name>/
        ├── vm.json          (VM config)
        ├── rootfs.ext4      (root filesystem)
        ├── cidata.img       (cloud-init CIDATA)
        └── console.log      (serial output)
```

## Development

```bash
# Build (requires Docker)
make docker-build   # Build dev image (once)
make build          # Compile to ./bin/fcm
make test           # Run tests

# Build the custom kernel
docker build -t fcm-kernel -f kernel/Dockerfile kernel/
```

## Custom Cloud-Init

Pass your own cloud-init config:

```bash
fcm create myvm --image ubuntu-24.04 --cloud-init ./my-config.yaml
```

Example templates are in the `env/` directory (Tailscale, Docker, etc).

## License

Apache 2.0
