#!/usr/bin/env python3
"""Generate deterministic WebP map pyramids and record them in the map manifest."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path

from PIL import Image, features


TILE_SIZE = 512
LEVELS = (1024, 2048, 4096, 8192)
WEBP_QUALITY = 82
REQUIRED_PILLOW_VERSION = "11.3.0"
REQUIRED_WEBP_VERSION = "1.5.0"


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def pyramid_is_current(directory: Path, manifest: dict[str, object], generator_hash: str) -> bool:
    if manifest.get("schemaVersion") != 2:
        return False
    expected_encoder = f"Pillow/{Image.__version__} libwebp/{features.version('webp')}"
    layers = manifest.get("layers")
    if not isinstance(layers, list):
        return False
    for layer in layers:
        if not isinstance(layer, dict):
            return False
        source_path = directory / str(layer.get("file", ""))
        if not source_path.is_file() or sha256(source_path) != layer.get("sha256"):
            return False
        pyramid = layer.get("tilePyramid")
        if not isinstance(pyramid, dict) or any(
            pyramid.get(key) != value
            for key, value in {
                "tileSize": TILE_SIZE,
                "format": "webp",
                "quality": WEBP_QUALITY,
                "method": 6,
                "exact": True,
                "resampling": "lanczos",
                "encoder": expected_encoder,
                "generatorSha256": generator_hash,
            }.items()
        ):
            return False
        levels = pyramid.get("levels")
        if not isinstance(levels, list) or len(levels) != len(LEVELS):
            return False
        aggregate_lines: list[str] = []
        for level_size, level in zip(LEVELS, levels):
            columns = level_size // TILE_SIZE
            if not isinstance(level, dict) or any(
                level.get(key) != value
                for key, value in {
                    "size": level_size,
                    "columns": columns,
                    "rows": columns,
                }.items()
            ):
                return False
            tiles = level.get("tiles")
            if not isinstance(tiles, list) or len(tiles) != columns * columns:
                return False
            layer_id = str(layer.get("id", ""))
            for index, tile in enumerate(tiles):
                x, y = index % columns, index // columns
                filename = f"{layer_id}-z{level_size}-x{x}-y{y}.webp"
                tile_path = directory / filename
                if (
                    not isinstance(tile, dict)
                    or tile.get("x") != x
                    or tile.get("y") != y
                    or tile.get("file") != filename
                    or not tile_path.is_file()
                    or tile_path.stat().st_size != tile.get("bytes")
                ):
                    return False
                tile_hash = sha256(tile_path)
                if tile_hash != tile.get("sha256"):
                    return False
                aggregate_lines.append(f"{level_size}/{x}/{y} {tile_hash}\n")
        aggregate = hashlib.sha256("".join(aggregate_lines).encode()).hexdigest()
        if aggregate != pyramid.get("sha256"):
            return False
    return True


def generate_layer(directory: Path, layer: dict[str, object], generator_hash: str) -> dict[str, object]:
    layer_id = str(layer["id"])
    source_path = directory / str(layer["file"])
    for stale_tile in directory.glob(f"{layer_id}-z*-x*-y*.webp"):
        stale_tile.unlink()
    with Image.open(source_path) as opened:
        source = opened.convert("RGB")
        if source.size != (LEVELS[-1], LEVELS[-1]):
            raise ValueError(f"{source_path} must be {LEVELS[-1]}x{LEVELS[-1]}, got {source.size}")

        generated_levels: list[dict[str, object]] = []
        aggregate_lines: list[str] = []
        for level_size in LEVELS:
            level = source if level_size == LEVELS[-1] else source.resize(
                (level_size, level_size), Image.Resampling.LANCZOS, reducing_gap=3.0
            )
            columns = level_size // TILE_SIZE
            tiles: list[dict[str, object]] = []
            for y in range(columns):
                for x in range(columns):
                    filename = f"{layer_id}-z{level_size}-x{x}-y{y}.webp"
                    destination = directory / filename
                    tile = level.crop(
                        (x * TILE_SIZE, y * TILE_SIZE, (x + 1) * TILE_SIZE, (y + 1) * TILE_SIZE)
                    )
                    tile.save(
                        destination,
                        format="WEBP",
                        quality=WEBP_QUALITY,
                        method=6,
                        exact=True,
                        exif=b"",
                        xmp=b"",
                        icc_profile=b"",
                    )
                    tile_hash = sha256(destination)
                    aggregate_lines.append(f"{level_size}/{x}/{y} {tile_hash}\n")
                    tiles.append(
                        {
                            "x": x,
                            "y": y,
                            "file": filename,
                            "bytes": destination.stat().st_size,
                            "sha256": tile_hash,
                        }
                    )
            generated_levels.append(
                {
                    "size": level_size,
                    "columns": columns,
                    "rows": columns,
                    "tiles": tiles,
                }
            )

    aggregate = hashlib.sha256("".join(aggregate_lines).encode()).hexdigest()
    return {
        "tileSize": TILE_SIZE,
        "format": "webp",
        "quality": WEBP_QUALITY,
        "method": 6,
        "exact": True,
        "resampling": "lanczos",
        "encoder": f"Pillow/{Image.__version__} libwebp/{features.version('webp')}",
        "generatorSha256": generator_hash,
        "sha256": aggregate,
        "levels": generated_levels,
    }


def main() -> None:
    if Image.__version__ != REQUIRED_PILLOW_VERSION:
        raise RuntimeError(
            f"Pillow {REQUIRED_PILLOW_VERSION} is required for deterministic map tiles; "
            f"found {Image.__version__}"
        )
    webp_version = features.version("webp")
    if webp_version != REQUIRED_WEBP_VERSION:
        raise RuntimeError(
            f"libwebp {REQUIRED_WEBP_VERSION} is required for deterministic map tiles; "
            f"found {webp_version}"
        )
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--if-needed",
        action="store_true",
        help="skip generation when every source, setting, and tile hash is current",
    )
    parser.add_argument(
        "directory",
        nargs="?",
        type=Path,
        default=Path(__file__).resolve().parents[1] / "assets" / "palworld" / "maps",
    )
    args = parser.parse_args()
    manifest_path = args.directory / "manifest.json"
    manifest = json.loads(manifest_path.read_text())
    generator_hash = sha256(Path(__file__).resolve())
    if args.if_needed and pyramid_is_current(args.directory, manifest, generator_hash):
        print("Map tiles are up to date.")
        return
    manifest["schemaVersion"] = 2
    for layer in manifest["layers"]:
        layer["tilePyramid"] = generate_layer(args.directory, layer, generator_hash)
    manifest_path.write_text(json.dumps(manifest, indent=2) + "\n")


if __name__ == "__main__":
    main()
