package server

import "embed"

// Vite build output should be written to internal/server/ui/dist.
//
//go:embed all:ui/dist
var spaAssets embed.FS
