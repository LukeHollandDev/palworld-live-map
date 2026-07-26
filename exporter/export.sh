#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repository_dir=$(CDPATH= cd -- "$script_dir/.." && pwd)

default_palworld_root="$HOME/Library/Application Support/CrossOver/Bottles/Steam/drive_c/Program Files (x86)/Steam/steamapps/common/Palworld"
palworld_root=${PALWORLD_ROOT:-$default_palworld_root}
pak_directory="$palworld_root/Pal/Content/Paks"

output_directory=${MAP_OUTPUT_DIR:-$repository_dir/build/maps}
landmark_output_directory=${LANDMARK_OUTPUT_DIR:-$repository_dir/build/landmarks}
game_version=${PALWORLD_GAME_VERSION-}

image_name=palworld-live-map/asset-exporter:dev

fail() {
  printf 'Error: %s\n' "$1" >&2
  exit 1
}

command -v docker >/dev/null 2>&1 || fail "Required command not found: docker"

if [ ! -d "$pak_directory" ]; then
  fail "Palworld PAK directory not found at $pak_directory. Set PALWORLD_ROOT to the directory containing Pal/ and Engine/."
fi

mkdir -p "$output_directory" "$landmark_output_directory"

printf 'Building the Palworld Asset Exporter...\n'
docker build --quiet -t "$image_name" "$script_dir" >/dev/null

printf 'Exporting map artwork and static world catalogue...\n'
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
  --mount "type=bind,src=$output_directory,dst=/output" \
  --mount "type=bind,src=$landmark_output_directory,dst=/landmark-output" \
  "$image_name" "$@"

printf 'Generated maps and provenance manifest in %s\n' "$output_directory"
printf 'Generated static world catalogue in %s\n' "$landmark_output_directory"
