package web

import "embed"

// Assets contains the browser application shipped inside the server binary.
//
//go:embed index.html app.js styles.css
var Assets embed.FS
