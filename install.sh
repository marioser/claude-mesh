#!/usr/bin/env bash
# Claude Mesh installer.
# Detects OS+arch, downloads the matching release tarball from GitHub,
# and installs binaries to $PREFIX/bin (default: ~/.local/bin).
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/marioser/claude-mesh/main/install.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/marioser/claude-mesh/main/install.sh | PREFIX=/usr/local bash
#   curl -fsSL https://raw.githubusercontent.com/marioser/claude-mesh/main/install.sh | VERSION=v0.1.0 bash

set -euo pipefail

REPO="marioser/claude-mesh"
PREFIX="${PREFIX:-${HOME}/.local}"
VERSION="${VERSION:-latest}"

bold() { printf "\033[1m%s\033[0m\n" "$*"; }
info() { printf "==> %s\n" "$*"; }
err()  { printf "error: %s\n" "$*" >&2; exit 1; }

# --- detect OS + arch -----------------------------------------------------
detect_os() {
    case "$(uname -s)" in
        Darwin) echo "macos" ;;
        Linux)  echo "linux" ;;
        *) err "unsupported OS: $(uname -s)" ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64) echo "x86_64" ;;
        arm64|aarch64) echo "arm64" ;;
        *) err "unsupported arch: $(uname -m)" ;;
    esac
}

# --- resolve version ------------------------------------------------------
resolve_version() {
    if [ "${VERSION}" != "latest" ]; then
        echo "${VERSION}"
        return
    fi
    local url="https://api.github.com/repos/${REPO}/releases/latest"
    local tag
    tag=$(curl -fsSL "${url}" | grep -E '"tag_name":' | head -1 | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
    if [ -z "${tag}" ]; then
        err "could not resolve latest version from GitHub API"
    fi
    echo "${tag}"
}

# --- sha256 helper --------------------------------------------------------
sha256() {
    if command -v sha256sum > /dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    elif command -v shasum > /dev/null 2>&1; then
        shasum -a 256 "$1" | awk '{print $1}'
    else
        err "neither sha256sum nor shasum is available"
    fi
}

# --- main -----------------------------------------------------------------
main() {
    bold "Claude Mesh installer"

    local os arch version tag_stripped
    os=$(detect_os)
    arch=$(detect_arch)
    version=$(resolve_version)
    tag_stripped="${version#v}"

    local archive="claude-mesh_${tag_stripped}_${os}_${arch}.tar.gz"
    local url="https://github.com/${REPO}/releases/download/${version}/${archive}"
    local checksum_url="https://github.com/${REPO}/releases/download/${version}/checksums.txt"

    info "OS:      ${os}"
    info "Arch:    ${arch}"
    info "Version: ${version}"
    info "Prefix:  ${PREFIX}"

    local tmp
    tmp=$(mktemp -d)
    trap 'rm -rf "${tmp}"' EXIT

    info "Downloading ${archive}"
    curl -fsSL -o "${tmp}/${archive}" "${url}"

    info "Verifying checksum"
    curl -fsSL -o "${tmp}/checksums.txt" "${checksum_url}"
    local expected actual
    expected=$(grep "${archive}" "${tmp}/checksums.txt" | awk '{print $1}')
    actual=$(sha256 "${tmp}/${archive}")
    if [ "${expected}" != "${actual}" ]; then
        err "checksum mismatch — expected ${expected}, got ${actual}"
    fi

    info "Extracting"
    tar -xzf "${tmp}/${archive}" -C "${tmp}"

    info "Installing to ${PREFIX}/bin"
    mkdir -p "${PREFIX}/bin"
    install -m 0755 "${tmp}/claude-mesh-bridge" "${PREFIX}/bin/claude-mesh-bridge"
    install -m 0755 "${tmp}/claude-mesh-mcp"    "${PREFIX}/bin/claude-mesh-mcp"

    bold "Installed:"
    echo "  ${PREFIX}/bin/claude-mesh-bridge"
    echo "  ${PREFIX}/bin/claude-mesh-mcp"
    echo
    echo "Next steps:"
    echo "  1. Ensure Mosquitto and Redis are running (or 'docker compose up -d' from the repo)."
    echo "  2. Run 'claude-mesh-bridge install' to wire up hooks, launchd, and MCP."
    echo "  3. Verify with 'claude-mesh-bridge status --json'."
}

main "$@"
