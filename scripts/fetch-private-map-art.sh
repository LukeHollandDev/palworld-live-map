#!/bin/sh
set -eu

destination=${1:-./maps}
mkdir -p "$destination"

# Pocketpair-derived 1.0 overview tiles are downloaded only to the operator's
# private runtime directory. They are intentionally excluded from Git/images.
curl --fail --location --silent --show-error \
  https://cdn.th.gl/palworld/map-tiles/default-733001e0986faa3f88b0a970412d7fb9/0/0/0.webp \
  --output "$destination/palpagos.webp"
curl --fail --location --silent --show-error \
  https://cdn.th.gl/palworld/map-tiles/tree-bd046c3cfb06ee41b25a111f912d407f/0/0/0.webp \
  --output "$destination/world-tree.webp"

echo "Private Palworld 1.0 overview maps installed in $destination"
