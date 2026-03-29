# FCM — Firecracker Machine Manager

## Design Document

### Overview

FCM is an open-source, single-binary CLI and API for managing Firecracker microVMs on bare metal Linux servers. It replaces the complexity of libvirt/QEMU with Firecracker's minimal footprint while providing a batteries-included experience: one curl command to install, auto-updates, cloud-init, Tailscale integration, and systemd-managed VMs.

FCM is the missing tool in the Firecracker ecosystem — the equivalent of what `virsh` is for libvirt, but purpose-built for Firecracker and designed for operators who want fast, disposable Linux VMs.

---

### Decisions

| Decision | Choice |
|---|---|
| Language | Go (single static binary) |
| VM access | Tailscale SSH + serial console fallback |
| State storage | JSON files per node |
| Process management | systemd unit per VM |
| Networking | Bridge + NAT (internet only) |
| Installation | `curl \| bash` one-liner |
| Auto-updates | Both fcm and Firecracker binaries via GitHub Releases |
| Image format | ext4 rootfs files |
| Cloud-init | NoCloud ISO attached as secondary drive |
| Primary goal | Replace GlueOps provisioner, then open source |

---

### Architecture

```
┌─────────────────────────────────────────────────────┐
│  Bare Metal Node                                    │
│                                                     │
│  ┌───────────┐                                      │
│  │  fcm CLI  │  ← operator runs commands here       │
│  └─────┬─────┘                                      │
│        │                                            │
│        ├── writes JSON state to /var/lib/fcm/vms/   │
│        ├── generates systemd units                  │
│        ├── creates TAP devices + cloud-init ISOs    │
│        └── calls systemctl start/stop               │
│                                                     │
│  systemd manages:                                   │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ │
│  │ fcm-vm-web   │ │ fcm-vm-api   │ │ fcm-vm-dev   │ │
│  │ firecracker  │ │ firecracker  │ │ firecracker  │ │
│  │ tap0 ──┐     │ │ tap1 ──┐     │ │ tap2 ──┐     │ │
│  └────────┼─────┘ └────────┼─────┘ └────────┼─────┘ │
│           │                │                │        │
│       ┌───┴────────────────┴────────────────┴───┐    │
│       │              fcbr0 (bridge)              │    │
│       │           192.168.100.1/24               │    │
│       └─────────────────┬────────────────────────┘    │
│                         │ NAT (iptables MASQUERADE)  │
│                         │                            │
│                    ┌────┴────┐                       │
│                    │  eth0   │  ← public interface   │
│                    └─────────┘                       │
└─────────────────────────────────────────────────────┘
```

---

### Installation

```bash
curl -fsSL https://get.fcm.dev | bash
```

The install script performs:

1. Detect architecture (x86_64 / aarch64)
2. Download `fcm` binary from GitHub Releases → `/usr/local/bin/fcm`
3. Download Firecracker binary from GitHub Releases → `/usr/local/bin/firecracker`
4. Download default kernel → `/var/lib/fcm/kernels/vmlinux-default`
5. Create directory structure:
   ```
   /var/lib/fcm/
   ├── config.json          # node-level config
   ├── kernels/
   │   └── vmlinux-default
   ├── images/              # rootfs images
   ├── vms/                 # per-VM state and data
   └── cache/               # downloaded image cache
   ```
6. Set up networking bridge (`fcbr0`) and NAT rules
7. Ensure `/dev/kvm` is accessible
8. Install systemd units:
   - `fcm-update.timer` — daily auto-update check
   - `fcm-update.service` — performs the update
9. Run `fcm init` to write initial config

**Post-install verification:**
```bash
fcm doctor
# ✓ /dev/kvm accessible
# ✓ Firecracker v1.12.1 installed
# ✓ Kernel available
# ✓ Bridge fcbr0 active
# ✓ NAT rules configured
# ✓ Auto-updates enabled
```

---

### CLI Interface

#### VM Lifecycle

```bash
# Create and start a VM
fcm create dev-box \
  --image ubuntu-22.04 \
  --cpus 2 \
  --memory 1024 \
  --disk 10 \
  --ssh-key ~/.ssh/id_ed25519.pub \
  --tailscale-key tskey-auth-xxx

# Create with custom cloud-init
fcm create dev-box \
  --image ubuntu-22.04 \
  --cpus 2 \
  --memory 1024 \
  --cloud-init ./my-cloud-config.yaml

# List all VMs
fcm list

# Output:
# NAME       STATUS    IP              CPUS  MEM    IMAGE         TAILSCALE
# dev-box    running   192.168.100.10  2     1024   ubuntu-22.04  connected
# worker-1   stopped   192.168.100.11  4     4096   ubuntu-22.04  -

# Start / stop / restart
fcm start dev-box
fcm stop dev-box
fcm restart dev-box

# Delete (also removes from Tailscale)
fcm delete dev-box

# SSH shortcut
fcm ssh dev-box

# Serial console (tails serial output)
fcm console dev-box

# View systemd logs
fcm logs dev-box
fcm logs dev-box --follow
```

#### VM Information

```bash
# Detailed VM info
fcm inspect dev-box

# Output (JSON):
# {
#   "name": "dev-box",
#   "status": "running",
#   "pid": 12345,
#   "ip": "192.168.100.10",
#   "tap_device": "tap0",
#   "mac": "AA:FC:00:00:00:01",
#   "cpus": 2,
#   "memory_mb": 1024,
#   "disk_gb": 10,
#   "image": "ubuntu-22.04",
#   "kernel": "vmlinux-default",
#   "created_at": "2026-03-29T10:00:00Z",
#   "tailscale_ip": "100.64.1.15",
#   "tags": {"owner": "john@example.com"}
# }

# Edit tags
fcm tag dev-box owner=john@example.com env=dev
```

#### Backups

```bash
# Disk backup (stops VM briefly)
fcm backup dev-box
fcm backup dev-box --output /backups/custom-path.ext4

# Full snapshot (sub-second pause, captures memory)
fcm snapshot dev-box

# List backups
fcm backups dev-box

# Restore
fcm restore dev-box dev-box-20260329-100000.ext4
fcm restore dev-box dev-box-20260328-040000.snap --snapshot

# Schedule nightly backups, keep 7 days
fcm backup schedule dev-box --interval daily --keep 7
```

#### Image Management

```bash
# List available images
fcm images

# Output:
# NAME            SIZE     SOURCE
# ubuntu-22.04    620 MB   github:glueops/vm-images
# ubuntu-24.04    680 MB   github:glueops/vm-images
# alpine-3.20     52 MB    github:glueops/vm-images

# Pull an image
fcm pull ubuntu-24.04

# Import a local image
fcm import my-image ./custom-rootfs.ext4

# Remove a cached image
fcm rmi alpine-3.20
```

#### System Management

```bash
# System health check
fcm doctor

# Node-level config
fcm config set bridge-subnet 192.168.100.0/24
fcm config set default-cpus 2
fcm config set default-memory 1024
fcm config get

# Self-update
fcm self-update
fcm self-update --version 0.5.0

# Update Firecracker (restarts all VMs)
fcm upgrade-firecracker
fcm upgrade-firecracker --version 1.13.0
```

---

### State Management

Each VM's state is a JSON file at `/var/lib/fcm/vms/{name}/vm.json`:

```json
{
  "name": "dev-box",
  "image": "ubuntu-22.04",
  "kernel": "vmlinux-default",
  "cpus": 2,
  "memory_mb": 1024,
  "disk_gb": 10,
  "ip": "192.168.100.10",
  "gateway": "192.168.100.1",
  "mac": "AA:FC:00:00:00:01",
  "tap_device": "tap0",
  "socket_path": "/var/lib/fcm/vms/dev-box/fc.socket",
  "rootfs_path": "/var/lib/fcm/vms/dev-box/rootfs.ext4",
  "cidata_path": "/var/lib/fcm/vms/dev-box/cidata.iso",
  "serial_log": "/var/lib/fcm/vms/dev-box/console.log",
  "tailscale_key": "tskey-auth-xxx",
  "tags": {
    "owner": "john@example.com"
  },
  "created_at": "2026-03-29T10:00:00Z",
  "boot_args": "console=ttyS0 reboot=k panic=1 ip=192.168.100.10::192.168.100.1:255.255.255.0::eth0:off"
}
```

**VM directory structure:**
```
/var/lib/fcm/vms/dev-box/
├── vm.json              # VM config and state
├── rootfs.ext4          # root filesystem (copy of image)
├── cidata.iso           # cloud-init ISO
├── fc.socket            # Firecracker API socket (runtime)
└── console.log          # serial output
```

**Node config** at `/var/lib/fcm/config.json`:

```json
{
  "bridge_name": "fcbr0",
  "bridge_ip": "192.168.100.1",
  "bridge_subnet": "192.168.100.0/24",
  "bridge_mask": "255.255.255.0",
  "ip_range_start": 10,
  "dns": "8.8.8.8",
  "default_kernel": "/var/lib/fcm/kernels/vmlinux-default",
  "default_cpus": 2,
  "default_memory_mb": 1024,
  "default_disk_gb": 10,
  "image_source": "github:glueops/vm-images",
  "auto_update": true,
  "fcm_version": "0.1.0",
  "firecracker_version": "1.12.1"
}
```

**IP allocation:** FCM scans existing `vm.json` files to find used IPs and allocates the next available from the range. No database needed.

---

### systemd Integration

#### Per-VM Unit (generated by `fcm create`)

`/etc/systemd/system/fcm-vm-dev-box.service`:

```ini
[Unit]
Description=FCM: dev-box (Firecracker microVM)
After=network.target fcm-bridge.service
Wants=fcm-bridge.service

[Service]
Type=simple
ExecStartPre=/usr/local/bin/fcm _setup-vm dev-box
ExecStart=/usr/local/bin/firecracker --api-sock /var/lib/fcm/vms/dev-box/fc.socket --log-path /var/lib/fcm/vms/dev-box/console.log
ExecStartPost=/usr/local/bin/fcm _configure-vm dev-box
ExecStopPost=/usr/local/bin/fcm _cleanup-vm dev-box
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
```

**Internal commands** (called by systemd, not by users):

- `fcm _setup-vm dev-box` — creates TAP device, attaches to bridge
- `fcm _configure-vm dev-box` — sends API calls to configure and boot the VM (kernel, drives, network, then InstanceStart)
- `fcm _cleanup-vm dev-box` — removes TAP device, cleans socket

#### Bridge Service

`/etc/systemd/system/fcm-bridge.service`:

```ini
[Unit]
Description=FCM: Network Bridge
After=network.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/local/bin/fcm _setup-bridge
ExecStop=/usr/local/bin/fcm _teardown-bridge

[Install]
WantedBy=multi-user.target
```

#### Auto-Update Timer

```ini
# /etc/systemd/system/fcm-update.timer
[Unit]
Description=FCM auto-update check

[Timer]
OnCalendar=*-*-* 04:00:00
Persistent=true
RandomizedDelaySec=1800

[Install]
WantedBy=timers.target
```

```ini
# /etc/systemd/system/fcm-update.service
[Unit]
Description=FCM auto-update

[Service]
Type=oneshot
ExecStart=/usr/local/bin/fcm self-update --auto
```

---

### Auto-Update Flow

#### FCM binary update (no VM restart):

1. Timer triggers `fcm self-update --auto`
2. Check latest GitHub Release tag for `fcm`
3. Compare with current version
4. If newer: download, verify checksum, replace `/usr/local/bin/fcm`
5. Log the update

#### Firecracker binary update (restarts all VMs):

1. `fcm self-update --auto` also checks Firecracker releases
2. If newer Firecracker version available:
   - Download and verify new binary
   - Get list of running VMs
   - `systemctl stop fcm-vm-*` (stop all VMs gracefully)
   - Replace `/usr/local/bin/firecracker`
   - `systemctl start fcm-vm-*` (restart all VMs)
   - Log the update with list of restarted VMs

**Safety:** Auto-update only runs during the configured maintenance window (default 04:00). A lockfile prevents concurrent updates. If any VM fails to restart, the update is logged as degraded and alerts via the configured webhook (optional).

---

### Networking

**Per-node setup (one-time, managed by `fcm _setup-bridge`):**

```
fcbr0 (bridge) — 192.168.100.1/24
  │
  ├── tap0 → VM dev-box  (192.168.100.10)
  ├── tap1 → VM worker-1 (192.168.100.11)
  └── tap2 → VM worker-2 (192.168.100.12)
  │
  └── NAT via iptables MASQUERADE → eth0 → internet
```

Each VM gets its IP configured via kernel boot args — no DHCP server needed. Guest networking is up before userspace starts.

**DNS inside VMs:** Configured via cloud-init to use the DNS server from node config (default `8.8.8.8`).

**VM isolation:** VMs on the same node can technically reach each other via the bridge. This is acceptable for the current use case. Future: add iptables rules to isolate VMs if needed.

---

### Cloud-Init

When `fcm create` is called with `--ssh-key`, `--tailscale-key`, or `--cloud-init`, FCM generates a NoCloud ISO:

**Default user-data (when using --ssh-key and --tailscale-key):**

```yaml
#cloud-config
hostname: dev-box
manage_etc_hosts: true
users:
  - name: root
    ssh_authorized_keys:
      - ssh-ed25519 AAAA...
package_update: true
packages:
  - curl
runcmd:
  - curl -fsSL https://tailscale.com/install.sh | sh
  - tailscale up --authkey=tskey-auth-xxx --ssh
  - echo "nameserver 8.8.8.8" > /etc/resolv.conf
```

**Custom cloud-init:** If `--cloud-init ./config.yaml` is provided, FCM uses it as-is, only injecting the hostname into meta-data.

**ISO generation:** FCM shells out to `genisoimage` (or `mkisofs`) to create the ISO. This is a build dependency installed during setup.

---

### Tailscale Integration

Tailscale is the primary access method for VMs.

**On create:**
- Tailscale auth key is embedded in cloud-init `runcmd`
- VM joins the tailnet on first boot
- `--ssh` flag enables Tailscale SSH (no need to manage SSH keys after initial setup)

**On delete:**
- FCM calls the Tailscale API to remove the device from the tailnet
- Requires `TAILSCALE_API_TOKEN` and `TAILSCALE_TAILNET` in node config

**SSH shortcut:**
```bash
fcm ssh dev-box
# Resolves to: ssh root@dev-box  (via Tailscale MagicDNS)
```

---

### Image Management

Images are ext4 rootfs files stored in `/var/lib/fcm/images/`.

**Image source:** GitHub Releases from a configurable repository (default: `glueops/vm-images`). Each release tag is an image version, with `.ext4` files as release assets.

**Pull flow:**
```
fcm pull ubuntu-22.04
  → GET https://api.github.com/repos/glueops/vm-images/releases/tags/ubuntu-22.04
  → Download ubuntu-22.04.ext4 asset
  → Save to /var/lib/fcm/images/ubuntu-22.04.ext4
```

**On VM create:**
```
cp --reflink=auto /var/lib/fcm/images/ubuntu-22.04.ext4 \
   /var/lib/fcm/vms/dev-box/rootfs.ext4
truncate -s 10G /var/lib/fcm/vms/dev-box/rootfs.ext4
resize2fs /var/lib/fcm/vms/dev-box/rootfs.ext4
```

Uses reflink copy on supporting filesystems (btrfs, XFS) for instant, space-efficient clones.

---

### Serial Console

Firecracker serial output is captured to `/var/lib/fcm/vms/{name}/console.log` via the `--log-path` flag.

```bash
# View console output (tail -f)
fcm console dev-box

# View last 50 lines
fcm console dev-box --lines 50
```

This is read-only — suitable for debugging boot issues. Interactive access is via Tailscale SSH.

---

### Backups

FCM supports two backup modes: disk-only (fast, simple) and full snapshots (memory + CPU + disk).

#### CLI

```bash
# Disk backup — stops VM, copies rootfs, restarts
fcm backup dev-box
fcm backup dev-box --output /backups/dev-box-2026-03-29.ext4

# Full snapshot — captures memory + CPU + disk state
fcm snapshot dev-box
fcm snapshot dev-box --output /backups/dev-box-snap/

# List backups for a VM
fcm backups dev-box

# Output:
# NAME                          TYPE      SIZE     CREATED
# dev-box-20260329-100000.ext4  disk      2.1 GB   2026-03-29 10:00:00
# dev-box-20260328-040000.snap  snapshot  3.4 GB   2026-03-28 04:00:00

# Restore from disk backup (replaces current rootfs)
fcm restore dev-box dev-box-20260329-100000.ext4

# Restore from snapshot (resumes exact state)
fcm restore dev-box dev-box-20260328-040000.snap --snapshot

# Delete a backup
fcm backup rm dev-box-20260329-100000.ext4
```

#### Disk Backup Flow

Simplest option — copies the rootfs file. On restore, the VM boots fresh but with all installed packages, files, and data intact.

```
fcm backup dev-box
  1. fcm stop dev-box              (systemctl stop fcm-vm-dev-box)
  2. cp rootfs.ext4 → /var/lib/fcm/backups/dev-box-{timestamp}.ext4
  3. cp vm.json → /var/lib/fcm/backups/dev-box-{timestamp}.json
  4. fcm start dev-box             (systemctl start fcm-vm-dev-box)

fcm restore dev-box {backup-file}
  1. fcm stop dev-box
  2. cp {backup-file} → /var/lib/fcm/vms/dev-box/rootfs.ext4
  3. fcm start dev-box
```

Downtime: a few seconds (copy time depends on disk size).

#### Full Snapshot Flow

Captures everything — memory, CPU registers, device state. The VM resumes exactly where it was: running processes, open files, network connections.

```
fcm snapshot dev-box
  1. Pause VM:
     PUT /vm → {"state": "Paused"}
  2. Create snapshot:
     PUT /snapshot/create → {
       "snapshot_type": "Full",
       "snapshot_path": "/var/lib/fcm/backups/dev-box-{timestamp}.snap",
       "mem_file_path": "/var/lib/fcm/backups/dev-box-{timestamp}.mem"
     }
  3. Copy rootfs to backup dir
  4. Copy vm.json to backup dir
  5. Resume VM:
     PATCH /vm → {"state": "Resumed"}

fcm restore dev-box {snapshot-dir} --snapshot
  1. fcm stop dev-box (if running)
  2. Copy rootfs from snapshot dir
  3. Start new Firecracker process
  4. Load snapshot:
     PUT /snapshot/load → {
       "snapshot_path": "{snapshot-dir}/snapshot.snap",
       "mem_backend": {
         "backend_path": "{snapshot-dir}/snapshot.mem",
         "backend_type": "File"
       }
     }
  5. Resume VM:
     PATCH /vm → {"state": "Resumed"}
```

Downtime: sub-second (VM is paused, not stopped).

**Important:** Snapshots are version-specific. A snapshot taken on Firecracker v1.12 cannot be restored on v1.13. FCM stores the Firecracker version in the backup metadata and blocks incompatible restores.

#### Scheduled Backups

Optional systemd timer per VM:

```bash
# Enable nightly disk backups
fcm backup schedule dev-box --interval daily --keep 7

# Disable
fcm backup unschedule dev-box
```

This generates:

```ini
# /etc/systemd/system/fcm-backup-dev-box.timer
[Unit]
Description=FCM: nightly backup for dev-box

[Timer]
OnCalendar=*-*-* 03:00:00
Persistent=true

[Install]
WantedBy=timers.target
```

```ini
# /etc/systemd/system/fcm-backup-dev-box.service
[Unit]
Description=FCM: backup dev-box

[Service]
Type=oneshot
ExecStart=/usr/local/bin/fcm backup dev-box --prune 7
```

The `--prune 7` flag deletes backups older than 7 days.

#### Backup Storage

```
/var/lib/fcm/backups/
├── dev-box-20260329-100000.ext4     # disk backup
├── dev-box-20260329-100000.json     # config snapshot
├── dev-box-20260328-040000/         # full snapshot dir
│   ├── snapshot.snap                # VM state
│   ├── snapshot.mem                 # memory dump
│   ├── rootfs.ext4                  # disk at snapshot time
│   └── vm.json                     # config at snapshot time
└── ...
```

Future: support pushing backups to S3/R2/B2 with `fcm backup dev-box --remote s3://bucket/path`.

---

### API (Phase 2)

A lightweight HTTP API for automation (Slackbot, CI). Runs as a separate binary or mode:

```bash
fcm serve --port 8080 --token mytoken
```

**Endpoints:**

```
GET    /v1/vms                  → fcm list (JSON)
GET    /v1/vms/{name}           → fcm inspect {name}
POST   /v1/vms                  → fcm create
POST   /v1/vms/{name}/start     → fcm start {name}
POST   /v1/vms/{name}/stop      → fcm stop {name}
DELETE /v1/vms/{name}           → fcm delete {name}
PUT    /v1/vms/{name}/tags      → fcm tag {name}
GET    /v1/images               → fcm images
POST   /v1/images/{name}/pull   → fcm pull {name}
GET    /v1/vms/{name}/backups  → fcm backups {name}
POST   /v1/vms/{name}/backup   → fcm backup {name}
POST   /v1/vms/{name}/snapshot → fcm snapshot {name}
POST   /v1/vms/{name}/restore  → fcm restore {name}
GET    /v1/health               → health check
```

Auth via `Authorization: Bearer <token>` header.

The API calls the same Go functions as the CLI — no duplication. This replaces the GlueOps provisioner's central FastAPI app. The Slackbot calls this API instead of SSHing into nodes.

---

### GlueOps Migration Path

**Current flow:**
```
Slackbot → GlueOps Provisioner (FastAPI) → SSH → virsh/virt-install on node
```

**New flow:**
```
Slackbot → FCM API (on each node, or a central proxy) → fcm CLI → Firecracker
```

**Migration steps:**

1. Install FCM on one bare metal node: `curl -fsSL https://get.fcm.dev | bash`
2. Pull your VM images: `fcm pull v0.76.0`
3. Test creating a VM: `fcm create test-vm --image v0.76.0 --tailscale-key tskey-xxx`
4. Verify Tailscale SSH works
5. Run `fcm serve` on the node
6. Point the Slackbot at the FCM API
7. Decommission the old provisioner
8. Repeat for remaining nodes

**What gets removed:**
- libvirt, QEMU, virt-install — no longer needed on nodes
- Apache Guacamole — replaced by Tailscale SSH
- The GlueOps provisioner FastAPI app — replaced by `fcm serve`
- SSH key management between provisioner and nodes — replaced by API auth

---

### Project Structure (Go)

```
fcm/
├── cmd/
│   └── fcm/
│       └── main.go              # CLI entrypoint (cobra)
├── internal/
│   ├── vm/
│   │   ├── create.go            # VM creation logic
│   │   ├── delete.go
│   │   ├── start.go
│   │   ├── stop.go
│   │   ├── list.go
│   │   ├── inspect.go
│   │   ├── backup.go            # Disk backup + full snapshots
│   │   └── state.go             # JSON state read/write
│   ├── firecracker/
│   │   └── client.go            # Unix socket API client
│   ├── network/
│   │   ├── bridge.go            # Bridge setup/teardown
│   │   ├── tap.go               # TAP device management
│   │   └── ip.go                # IP allocation
│   ├── cloudinit/
│   │   └── iso.go               # Cloud-init ISO generation
│   ├── images/
│   │   └── manager.go           # Image pull/list/import
│   ├── tailscale/
│   │   └── client.go            # Tailscale API for cleanup
│   ├── update/
│   │   └── updater.go           # Self-update + Firecracker update
│   ├── systemd/
│   │   └── unit.go              # Generate/manage systemd units
│   └── config/
│       └── config.go            # Node config management
├── api/
│   └── server.go                # HTTP API server
├── install.sh                   # curl | bash installer
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

### Dependencies

**On the node (installed by setup script):**
- `/dev/kvm` (hardware virtualization)
- `genisoimage` or `mkisofs` (cloud-init ISO creation)
- `iptables` (NAT rules)
- `iproute2` (bridge and TAP management)
- systemd

**Go dependencies (compiled into the binary):**
- `cobra` — CLI framework
- `gin` or `chi` — HTTP API
- Standard library for Unix socket, JSON, HTTP

**No runtime dependencies:** FCM is a static binary. Firecracker is a static binary. Everything else is standard Linux.

---

### Future Considerations

Not in scope for v1, but designed to be possible:

- **Web UI** — small dashboard showing VM status across nodes
- **Multi-node coordination** — central API that routes to per-node FCM instances
- **VM templates** — preconfigured VM definitions (`fcm create --template k8s-worker`)
- **Metrics** — expose Firecracker's metrics endpoint via `fcm metrics`
- **VM isolation** — iptables rules to prevent VM-to-VM traffic on the bridge
- **Webhook notifications** — notify on VM events (created, crashed, updated)
- **Remote backups** — push backups to S3/R2/B2 with `fcm backup --remote`
