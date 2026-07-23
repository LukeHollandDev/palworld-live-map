#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repository_dir=$(CDPATH= cd -- "$script_dir/.." && pwd)

default_palworld_root="$HOME/Library/Application Support/CrossOver/Bottles/Steam/drive_c/Program Files (x86)/Steam/steamapps/common/Palworld"
palworld_root=${PALWORLD_ROOT:-$default_palworld_root}
pak_directory="$palworld_root/Pal/Content/Paks"

output_directory=${MAP_OUTPUT_DIR:-$repository_dir/build/maps}
landmark_output_directory=${LANDMARK_OUTPUT_DIR:-$repository_dir/build/landmarks}
cache_directory=${ASSET_EXPORT_CACHE_DIR:-$repository_dir/build/asset-exporter-cache}
game_version=${PALWORLD_GAME_VERSION-}

mappings_commit=42cf396e714c166f17950a9c964583e0cadf2a15
mappings_sha256=241c45de9d5b55b246cd4b39d62b9209faf7758ce0637e1f7a545aa0f75f71f0
mappings_file="$cache_directory/Mappings-$mappings_commit.usmap"
mappings_url="https://raw.githubusercontent.com/PalworldModding/UsefulFiles/$mappings_commit/Mappings.usmap"

image_name=palworld-live-map/asset-exporter:dev

fail() {
  printf 'Error: %s\n' "$1" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "Required command not found: $1"
}

check_requirements() {
  require_command awk
  require_command curl
  require_command docker
  require_command shasum

  game_version=$(printf '%s\n' "$game_version" | awk '{$1=$1; print}')
  if [ -n "$game_version" ] && ! printf '%s\n' "$game_version" | awk '/^[0-9]+\.[0-9]+\.[0-9]+(\.[0-9]+)?$/ { valid = 1 } END { exit !valid }'; then
    fail "PALWORLD_GAME_VERSION is an optional exact-version assertion and must contain three or four numeric components."
  fi

  if [ ! -d "$pak_directory" ]; then
    fail "Palworld PAK directory not found at $pak_directory. Set PALWORLD_ROOT to the directory containing Pal/ and Engine/."
  fi
}

prepare_mappings() {
  mkdir -p "$cache_directory" "$output_directory" "$landmark_output_directory"

  if [ ! -f "$mappings_file" ]; then
    printf 'Downloading Palworld mappings...\n'
    curl -fsSL "$mappings_url" -o "$mappings_file"
  fi

  actual_sha256=$(shasum -a 256 "$mappings_file" | awk '{print $1}')
  if [ "$actual_sha256" != "$mappings_sha256" ]; then
    fail "Mappings checksum mismatch: got $actual_sha256"
  fi
}

build_exporter() {
  printf 'Building the Palworld Asset Exporter...\n'
  docker build --quiet -t "$image_name" "$script_dir" >/dev/null
}

run_exporter() {
  printf 'Exporting map artwork and encounter landmarks...\n'
  set -- \
    --pak-directory /palworld-paks \
    --mappings /mappings.usmap \
    --output /output \
    --landmark-output /landmark-output
  if [ -n "$game_version" ]; then
    set -- "$@" --game-version "$game_version"
  fi
  docker run --rm \
    --mount "type=bind,src=$pak_directory,dst=/palworld-paks,readonly" \
    --mount "type=bind,src=$mappings_file,dst=/mappings.usmap,readonly" \
    --mount "type=bind,src=$output_directory,dst=/output" \
    --mount "type=bind,src=$landmark_output_directory,dst=/landmark-output" \
    "$image_name" "$@"
}

check_requirements
prepare_mappings
build_exporter
run_exporter

printf 'Generated maps and provenance manifest in %s\n' "$output_directory"
printf 'Generated encounter landmark manifest in %s\n' "$landmark_output_directory"
