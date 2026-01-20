// Package buildinfo holds build-time metadata injected via -ldflags.
package buildinfo

// Version is the semantic version or tag for this build.
// Inject via: -X github.com/garyellow/ntpu-linebot-go/internal/buildinfo.Version=...
var Version = ""

// Commit is the git commit SHA for this build.
// Inject via: -X github.com/garyellow/ntpu-linebot-go/internal/buildinfo.Commit=...
var Commit = ""

// BuildDate is the RFC3339 build timestamp.
// Inject via: -X github.com/garyellow/ntpu-linebot-go/internal/buildinfo.BuildDate=...
var BuildDate = ""
