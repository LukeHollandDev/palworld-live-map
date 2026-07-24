package assets

import "embed"

// Maps contains the terrain artwork shipped with the application.
//
//go:embed palworld/maps/*.jpg palworld/maps/manifest.json
var Maps embed.FS

// Landmarks contains versioned, non-player encounter locations extracted
// from Palworld's game data. They are kept separate from live REST snapshots
// because unloaded encounters do not reliably appear in the API.
//
//go:embed palworld/landmarks/manifest.json
var Landmarks embed.FS
