#!/usr/bin/env bash
# rook installer
#
#   curl -fsSL https://github.com/pdparchitect/rook/releases/latest/download/install.sh | bash
#
# Installs the latest rook release for this machine. Downloads the platform
# tarball from the pdparchitect/rook GitHub release, verifies it against the
# release's checksums.txt, and puts the binary in ~/.local/bin.
#
# This script is the source of truth: it ships in the rook repository and is
# published as an asset on every release, so the URL above always serves the
# current installer.
#
# Options (environment variables, or a version as the first argument):
#   ROOK_VERSION      release to install - "latest" (default) or a tag such as
#                     vX.Y.Z / X.Y.Z. Also: curl ... | bash -s -- vX.Y.Z
#   ROOK_INSTALL_DIR  where to put the binary (default: ~/.local/bin)
#
# Linux and macOS, amd64 and arm64. On Windows, download the archive from
# https://github.com/pdparchitect/rook/releases instead.
set -euo pipefail

main() {
  local repo="pdparchitect/rook"
  local requested="${1:-${ROOK_VERSION:-latest}}"
  local install_dir="${ROOK_INSTALL_DIR:-${HOME}/.local/bin}"

  log() { echo "rook: $*" >&2; }
  fail() { log "$*"; exit 1; }

  command -v curl >/dev/null 2>&1 || fail "curl is required"
  command -v tar >/dev/null 2>&1 || fail "tar is required"

  local os arch
  case "$(uname -s)" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    MINGW* | MSYS* | CYGWIN*)
      fail "Windows is not supported by this script - download rook from https://github.com/${repo}/releases" ;;
    *) fail "unsupported OS: $(uname -s)" ;;
  esac

  case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) fail "unsupported architecture: $(uname -m)" ;;
  esac

  # Resolve "latest" through the release redirect - no API call, no rate limit.
  local version
  if [[ "$requested" == "latest" ]]; then
    local location
    location="$(curl -fsSI -o /dev/null -w '%{redirect_url}' "https://github.com/${repo}/releases/latest")"
    version="${location##*/}"
    [[ "$version" =~ ^v[0-9] ]] || fail "could not resolve the latest release from ${location:-<no redirect>}"
  else
    version="v${requested#v}"
  fi

  local root="rook-${version}-${os}-${arch}"
  local archive="${root}.tar.gz"
  local base="https://github.com/${repo}/releases/download/${version}"

  # not local: the EXIT trap runs after main has returned
  work="$(mktemp -d)"
  trap 'rm -rf "$work"' EXIT

  log "downloading ${archive}"
  curl -fsSL --retry 3 -o "${work}/${archive}" "${base}/${archive}" \
    || fail "no release asset ${archive} - is ${version} a real release, and ${os}/${arch} a published platform?"
  curl -fsSL --retry 3 -o "${work}/checksums.txt" "${base}/checksums.txt" \
    || fail "could not download checksums.txt for ${version}"

  local expected actual
  expected="$(grep -E "[[:space:]]\*?${archive}\$" "${work}/checksums.txt" | awk '{print $1}' | head -n1)"
  [[ -n "$expected" ]] || fail "checksums.txt has no entry for ${archive}"

  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "${work}/${archive}" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "${work}/${archive}" | awk '{print $1}')"
  else
    fail "need sha256sum or shasum to verify the download"
  fi
  [[ "$expected" == "$actual" ]] || fail "checksum mismatch for ${archive}: expected ${expected}, got ${actual}"

  tar -xzf "${work}/${archive}" -C "$work"
  mkdir -p "$install_dir"
  install -m 0755 "${work}/${root}/rook" "${install_dir}/rook"

  log "installed rook ${version} (${os}/${arch}) to ${install_dir}/rook"
  "${install_dir}/rook" --version >&2

  case ":${PATH}:" in
    *":${install_dir}:"*) ;;
    *)
      log ""
      log "${install_dir} is not on your PATH. Add it, e.g.:"
      log "  export PATH=\"${install_dir}:\$PATH\""
      ;;
  esac

  log ""
  log "next: export a provider key and hand rook an objective -"
  log "  export ZAI_API_KEY=\"...\""
  log "  rook new \"audit the auth handlers for bypass bugs\""
  log "  rook"
}

main "$@"
