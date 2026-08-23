#!/usr/bin/env sh
set -eu

version="${1:-}"
if ! printf '%s\n' "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "usage: $0 vMAJOR.MINOR.PATCH" >&2
  exit 2
fi

image="muni:${version}"
archive="release/muni-${version}.tar.gz"
commit="${GITHUB_SHA:-none}"
build_time="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

mkdir -p release
docker build \
  --build-arg "VERSION=${version}" \
  --build-arg "COMMIT=${commit}" \
  --build-arg "BUILD_TIME=${build_time}" \
  --tag "${image}" .
docker image inspect "${image}" >/dev/null
docker save "${image}" | gzip -9 > "${archive}"
echo "created ${archive} containing ${image}"
