// Package version carries build metadata injected at link time.
package version

// These values are overridden through -ldflags at build time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// String returns a human readable build identifier.
func String() string {
	return Version + " (" + Commit + ", " + Date + ")"
}
