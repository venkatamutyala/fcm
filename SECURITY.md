# Security Policy

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, email security reports to: **security@fcm.dev**

Include as much of the following as possible:

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

You will receive an acknowledgment within 48 hours and a detailed response within 7 days indicating next steps.

## Scope

fcm has a significant security surface area. The tool:

- **Runs as root** to manage Firecracker microVMs
- **Manages iptables rules** for VM networking (NAT, forwarding)
- **Creates and manages tap devices** for VM network interfaces
- **Manages systemd services** for VM lifecycle
- **Downloads and executes binaries** (Firecracker, kernel, rootfs images)
- **Generates SSH keys** and manages VM access
- **Configures cloud-init** with user data

Security issues in any of these areas are in scope.

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| latest  | Yes                |
| < 0.1.0 | No                |

## Response Timeline

| Action                  | Timeline   |
| ----------------------- | ---------- |
| Acknowledgment          | 48 hours   |
| Initial assessment      | 7 days     |
| Fix development         | 30 days    |
| Public disclosure        | After fix  |

## Disclosure Policy

We follow coordinated disclosure. We ask that you:

1. Give us reasonable time to fix the issue before public disclosure.
2. Make a good faith effort to avoid data destruction and service disruption.
3. Do not access or modify data belonging to others.

We will credit reporters in the release notes (unless you prefer to remain anonymous).
