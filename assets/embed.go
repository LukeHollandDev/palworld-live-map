package assets

import "embed"

// Maps contains the terrain artwork shipped with the application.
//
//go:embed map/*.jpg map/manifest.json
var Maps embed.FS
