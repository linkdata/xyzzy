package xyzzy

import "embed"

// Assets contains the application's embedded templates, static files and deck data.
//
//go:embed assets
var Assets embed.FS
