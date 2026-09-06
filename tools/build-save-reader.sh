#!/bin/sh
set -eu

reader_version=v0.3.0
reader_revision=922c229292277ad239507d4b2ae0eb75d8b0ac64

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

mkdir -p "$project_root/bin"
make -C "$reader_stage/source" release-build \
  GOOS="$(go env GOOS)" \
  GOARCH="$(go env GOARCH)" \
  VERSION="$reader_version" \
  OUTPUT="$project_root/bin/palworld-save-reader"
