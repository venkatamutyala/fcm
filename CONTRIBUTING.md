# Contributing to fcm

Thanks for your interest in contributing to fcm! This guide will help you get started.

## Prerequisites

- Docker (for building)
- Linux host (for testing VMs)
- Go 1.23+ (optional, if building outside Docker)

## Development Setup

fcm uses a Docker-based development workflow so you do not need Go installed locally.

### Build the dev container

```bash
make docker-build
```

### Build the binary

```bash
make build
```

The binary is output to `./bin/fcm`.

### Interactive development shell

```bash
make docker-shell
```

This drops you into a container with Go and all build tools available.

## Testing

Run the full test suite:

```bash
make test
```

Tests run inside Docker with `-race` enabled.

## Building the Kernel

To build the custom Linux kernel used by fcm:

```bash
docker build -t fcm-kernel -f kernel/Dockerfile kernel/
```

## Code Style

- Format code with `go fmt ./...`
- Run the linter before submitting: `make lint`
- The linter uses [golangci-lint](https://golangci-lint.run/) v1.62

## Pull Request Process

1. Fork the repository and create a feature branch from `main`.
2. Make your changes. Add or update tests as appropriate.
3. Ensure all checks pass: `make lint && make test && make build`
4. Write a clear commit message describing what and why.
5. Open a pull request against `main`.
6. A maintainer will review your PR. Address any feedback.
7. Once approved, a maintainer will merge your PR.

## Reporting Issues

- **Bugs**: Use the [bug report template](https://github.com/venkatamutyala/fcm/issues/new?template=bug_report.md).
- **Features**: Use the [feature request template](https://github.com/venkatamutyala/fcm/issues/new?template=feature_request.md).
- **Security**: See [SECURITY.md](SECURITY.md) for responsible disclosure.

## Project Structure

```
cmd/fcm/          CLI commands (Cobra)
internal/         Internal packages
  cloudinit/      Cloud-init configuration
  config/         Configuration management
  firecracker/    Firecracker process management
  images/         Image management (kernel, rootfs)
  network/        Networking (tap devices, iptables)
  systemd/        systemd service management
  vm/             VM lifecycle
kernel/           Custom kernel build
```

## License

By contributing, you agree that your contributions will be licensed under the project's license.
