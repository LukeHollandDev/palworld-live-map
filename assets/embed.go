package assets

import "embed"

// Maps contains the terrain artwork shipped with the application.
//
//go:embed map/*.jpg map/manifest.json
var Maps embed.FS

// Landmarks contains versioned, non-player encounter locations extracted
// from Palworld's game data. They are kept separate from live REST/save
// snapshots because unloaded encounters do not reliably appear in either.
//
//go:embed landmarks/manifest.json
var Landmarks embed.FS
