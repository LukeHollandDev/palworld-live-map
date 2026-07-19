package web

import "embed"

// Assets contains the browser application shipped inside the server binary.
//
//go:embed all:dist
var Assets embed.FS
