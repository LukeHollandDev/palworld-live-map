#!/bin/sh
set -eu

reader_version=v0.2.0
reader_revision=c6560931f407abcbe3398a3fc73840b51bb56974
reader_build_version="${reader_version}+live-map.1"

project_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
reader_stage=$(mktemp -d "${TMPDIR:-/tmp}/palworld-live-map-reader.XXXXXX")
trap 'rm -rf -- "$reader_stage"' EXIT HUP INT TERM

git clone --branch "$reader_version" --depth 1 \
  https://github.com/LukeHollandDev/palworld-save-reader.git "$reader_stage/source"
actual_revision=$(git -C "$reader_stage/source" rev-parse HEAD)
if [ "$actual_revision" != "$reader_revision" ]; then
  printf 'save-reader revision mismatch: expected %s, got %s\n' \
    "$reader_revision" "$actual_revision" >&2
  exit 1
fi
git -C "$reader_stage/source" apply --check \
  "$project_root/patches/palworld-save-reader-v0.2.0-leaderboards.patch"
git -C "$reader_stage/source" apply \
  "$project_root/patches/palworld-save-reader-v0.2.0-leaderboards.patch"

mkdir -p "$project_root/bin"
make -C "$reader_stage/source" release-build \
  GOOS="$(go env GOOS)" \
  GOARCH="$(go env GOARCH)" \
  VERSION="$reader_build_version" \
  OUTPUT="$project_root/bin/palworld-save-reader"
