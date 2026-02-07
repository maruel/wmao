// Package frontend embeds the built frontend assets.
package frontend

import "embed"

//go:embed dist

// Files contains the built frontend assets.
var Files embed.FS
