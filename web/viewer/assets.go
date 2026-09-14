package viewer

import "embed"

// Files embeds the frontend distribution. Build assets before compiling Go.
// Bootstrap must serve fs.Sub(Files, "assets"), never the repository directory.
//
//go:embed assets
var Files embed.FS
