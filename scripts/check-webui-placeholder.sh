#!/usr/bin/env bash
# webui/dist holds a placeholder index.html so `go build` works in a checkout
# where the frontend has never been built. `npm run build` overwrites it, and
# every file it writes beside it is gitignored — so committing the built page
# leaves an index.html pointing at assets nobody else has.
#
# Usage: check-webui-placeholder.sh [--staged]
set -euo pipefail

path="webui/dist/index.html"
cd "$(git rev-parse --show-toplevel)"

if [ "${1:-}" = "--staged" ]; then
  content="$(git show ":$path" 2>/dev/null || true)"
else
  content="$(cat "$path" 2>/dev/null || true)"
fi

if [ -z "$content" ]; then
  echo "$path is missing; the Go binary embeds webui/dist and cannot build without it." >&2
  exit 1
fi

if printf '%s' "$content" | grep -q '/assets/'; then
  cat >&2 <<'MESSAGE'
webui/dist/index.html holds a built bundle instead of the placeholder.

Only the placeholder belongs in git. The asset files vite writes next to it are
ignored, so committing the built page ships an index.html that references files
that are not in the repository.

Restore it with:

    git checkout -- webui/dist/index.html

Docker and the release workflow build the real bundle themselves.
MESSAGE
  exit 1
fi
