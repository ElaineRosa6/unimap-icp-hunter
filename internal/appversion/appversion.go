package appversion

import "fmt"

// These variables can be overridden by -ldflags at build time.
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

func Short() string {
	return Version
}

func Full() string {
	return fmt.Sprintf("%s (commit=%s, built=%s)", Version, GitCommit, BuildTime)
}
