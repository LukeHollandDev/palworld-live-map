package web

import "embed"

// Assets contains the browser application shipped inside the server binary.
// Map artwork is intentionally not embedded; operators mount their own files.
//
//go:embed index.html app.js styles.css
var Assets embed.FS
