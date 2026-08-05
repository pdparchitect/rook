#!/bin/bash
# Static checks for the Rook desktop image. No container is built here; the live
# behaviour is asserted against a running session by
# overlay/usr/local/bin/desktop-selftest under the substrate's smoke test.

set -euo pipefail

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
repo_root="$(cd "$project_dir/../.." && pwd)"
dockerfile="$project_dir/Dockerfile"

# Every shell program shipped in the overlay, plus the theme script, must parse.
mapfile -t shell_sources < <(
    find "$project_dir/overlay/usr/local/bin" "$project_dir/theme" \
        "$project_dir/overlay/etc" -type f 2>/dev/null | sort
)
if [ "${#shell_sources[@]}" -gt 0 ]; then
    bash -n "${shell_sources[@]}"
fi

# The binary is compiled from this repo, never downloaded from a release: that
# is the whole reason the image lives beside the source. Guard against a
# regression that reintroduces a floating download.
if grep -Eiq 'releases/download|ROOK_SHA256|ROOK_VERSION' "$dockerfile"; then
    echo "Dockerfile downloads a Rook release; build from source instead." >&2
    exit 1
fi
grep -Fq 'go build' "$dockerfile"

# The persistent config dir and the exported-key hook have to name the same
# path, or a saved key never reaches rook.
grep -Fq '/home/agent/.config/rook' "$dockerfile"
grep -Fq '.config/rook/env' \
    "$project_dir/overlay/etc/profile.d/rook-env.sh"

# The application manifest must be valid JSON when a parser is available.
manifest="$project_dir/launcher/application.json"
if command -v python3 >/dev/null 2>&1; then
    python3 -m json.tool "$manifest" >/dev/null
fi

# The build context reaches the Go module and the overlay; go.work must not,
# because it points at a local ../go-sdk checkout absent from the build.
grep -Fxq 'go.work' "$repo_root/.dockerignore"

echo "Rook desktop image checks passed."
