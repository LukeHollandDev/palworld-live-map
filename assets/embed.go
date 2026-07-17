package assets

import "embed"

// Maps contains the terrain artwork shipped with the application.
//
//go:embed map/*.jpg
var Maps embed.FS
