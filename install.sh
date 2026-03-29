#!/usr/bin/env bash
#
# fcm installer - download and install the latest fcm release
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/venkatamutyala/fcm/main/install.sh | bash
#
set -euo pipefail

REPO="venkatamutyala/fcm"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="fcm"

info()  { echo "[fcm] $*"; }
error() { echo "[fcm] ERROR: $*" >&2; exit 1; }

# Detect architecture
detect_arch() {
  local arch
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64)  echo "amd64" ;;
    aarch64|arm64)  echo "arm64" ;;
    *) error "Unsupported architecture: $arch" ;;
  esac
}

# Detect OS
detect_os() {
  local os
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$os" in
    linux) echo "linux" ;;
    *) error "Unsupported OS: $os. fcm only supports Linux." ;;
  esac
}

# Get the latest release tag from GitHub
get_latest_version() {
  local version
  version="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | head -1 | cut -d '"' -f 4)"
  if [ -z "$version" ]; then
    error "Could not determine latest version. Check https://github.com/${REPO}/releases"
  fi
  echo "$version"
}

main() {
  local os arch version archive_name download_url checksum_url tmp_dir

  os="$(detect_os)"
  arch="$(detect_arch)"
  version="$(get_latest_version)"

  info "Installing fcm ${version} (${os}/${arch})"

  archive_name="fcm_${version#v}_${os}_${arch}.tar.gz"
  download_url="https://github.com/${REPO}/releases/download/${version}/${archive_name}"
  checksum_url="https://github.com/${REPO}/releases/download/${version}/checksums.txt"

  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "$tmp_dir"' EXIT

  info "Downloading ${download_url}"
  curl -fsSL -o "${tmp_dir}/${archive_name}" "$download_url"

  info "Downloading checksums"
  curl -fsSL -o "${tmp_dir}/checksums.txt" "$checksum_url"

  info "Verifying SHA256 checksum"
  (cd "$tmp_dir" && grep "${archive_name}" checksums.txt | sha256sum -c --quiet -) \
    || error "Checksum verification failed"

  info "Extracting binary"
  tar -xzf "${tmp_dir}/${archive_name}" -C "$tmp_dir" "$BINARY_NAME"

  info "Installing to ${INSTALL_DIR}/${BINARY_NAME}"
  if [ -w "$INSTALL_DIR" ]; then
    install -m 0755 "${tmp_dir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
  else
    info "Elevated permissions required to install to ${INSTALL_DIR}"
    sudo install -m 0755 "${tmp_dir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
  fi

  info "Successfully installed fcm ${version} to ${INSTALL_DIR}/${BINARY_NAME}"
  echo ""
  echo "  Get started:"
  echo "    fcm init      Initialize fcm (downloads kernel, rootfs, Firecracker)"
  echo "    fcm create     Create your first microVM"
  echo "    fcm list       List running VMs"
  echo ""
  echo "  Run 'fcm init' to get started."
}

main
