package version

// Version is set at build time via -ldflags "-X github.com/ryotarai/arca/internal/version.Version=..."
// When not set (local development), it defaults to "dev".
var Version = "dev"
