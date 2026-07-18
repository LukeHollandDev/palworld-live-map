# Map artwork

`palpagos.jpg` and `world-tree.jpg` are 8192×8192 overview maps assembled from zoom-level 4 tiles served by THGL's Palworld map CDN. They replace the former 512×512 runtime downloads so every repository checkout and container image has useful terrain detail without an operator-managed volume.

The coordinate orientation and world bounds are defined in `internal/server/server.go`. Palworld and the source artwork are owned by Pocketpair; the project MIT license applies only to the project's original code and documentation.
