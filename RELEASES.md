# Releasing Rook

This document describes how to build, version, and release Rook binaries.

## Overview

Rook uses **Git tags** to trigger automated releases. When a tag matching `v*`
is pushed to the `pdparchitect/rook` repository, a GitHub Actions workflow builds
multi-platform binaries and publishes them as a GitHub Release.

### Engine dependency

The committed `go.mod` pins a **tagged release** of the engine,
`github.com/openzot/openzot` (zot), so every build - clean clone, CI, release,
and `go install` - uses exactly that version. Builds are reproducible; no
floating fetch step is involved.

To move to a newer engine, bump the pin explicitly and commit the result:

```bash
go get github.com/openzot/openzot@vX.Y.Z
go mod tidy
```

## Version embedding

Every binary embeds a version string at build time via Go linker flags. The
version variable lives in `internal/version/version.go` and defaults to `"dev"`
when no flag is set (e.g. when running with `go run`). When the version is
`"dev"`, update checks are skipped entirely.

## How to release

### 1. Tag the release

```bash
git tag v0.1.0
git push origin v0.1.0
```

Use semantic versioning: `vMAJOR.MINOR.PATCH` (with the `v` prefix).

### 2. Wait for CI

The [release workflow](.github/workflows/release.yaml) runs automatically and:

1. Resolves the pinned engine module.
2. Builds the `rook` binary for each target platform.
3. Packages each into a `.tar.gz` archive (with README and LICENSE).
4. Generates SHA-256 checksums.
5. Creates a GitHub Release with auto-generated notes.

### Target platforms

| OS      | Architecture |
| ------- | ------------ |
| Linux   | amd64, arm64 |
| macOS   | amd64, arm64 |
| Windows | amd64        |

## Local builds

```bash
# Build the binary (version auto-detected from git tags)
make build

# Build with an explicit version
make build VERSION=v0.1.0

# Cross-compile a single platform
make cross GOOS=darwin GOARCH=arm64

# Build all release archives under dist/
make dist VERSION=v0.1.0

# Run tests
make test
```

## Update notifications

Release binaries check for newer versions by querying the GitHub Releases API
on `rook version` and after a run. The check is **skipped entirely** for `dev`
builds, so it only applies to distributed binaries.

## Versioning guidelines

- Follow [Semantic Versioning](https://semver.org/).
- Use a `v` prefix on tags (`v1.0.0`, not `1.0.0`).
- Pre-release versions: `v0.1.0-beta.1`.
