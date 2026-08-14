package assets

import "embed"

// Maps contains the terrain artwork shipped with the application.
//
// Generated WebP tiles are intentionally absent from Git. Supported build
// workflows create them beside the source artwork before compiling this tree.
//
//go:embed palworld/maps
var Maps embed.FS

// Landmarks contains versioned, non-player encounter locations extracted
// from Palworld's game data. They are kept separate from live REST snapshots
// because unloaded encounters do not reliably appear in the API.
//
//go:embed palworld/landmarks/manifest.json
var Landmarks embed.FS

//go:embed palworld/landmarks/catalogue/*.json
var Catalogue embed.FS
