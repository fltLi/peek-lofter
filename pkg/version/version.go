package version

// Build metadata injected with -ldflags at release time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)
