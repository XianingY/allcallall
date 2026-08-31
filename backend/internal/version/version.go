// Package version exposes build/version metadata for the SLA status page and
// operational tooling. Values are overridden at build time via -ldflags, e.g.
//
//	go build -ldflags "-X github.com/allcallall/backend/internal/version.Version=1.4.2 \
//	  -X github.com/allcallall/backend/internal/version.Commit=$(git rev-parse --short HEAD) \
//	  -X github.com/allcallall/backend/internal/version.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
//
// A runtime override via the ALLCALLALL_VERSION env var ("version|commit|builddate")
// is also supported for immutable images stamped at deploy time.
package version

import (
	"os"
	"strings"
	"sync"
)

// Build information. Override via -ldflags "-X ...version.Version=...".
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
	GoVersion = "unknown"
)

// envOverrideKey allows deploy-time stamping of an otherwise dev image.
const envOverrideKey = "ALLCALLALL_VERSION"

var (
	once     sync.Once
	resolved Info
)

// Info holds resolved build/version metadata.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
}

// Get returns resolved version info, preferring any runtime env override.
func Get() Info {
	once.Do(func() {
		resolved = Info{
			Version:   Version,
			Commit:    Commit,
			BuildDate: BuildDate,
			GoVersion: GoVersion,
		}
		if v := os.Getenv(envOverrideKey); v != "" {
			// format: version|commit|builddate
			parts := strings.Split(v, "|")
			if p := strings.TrimSpace(parts[0]); p != "" {
				resolved.Version = p
			}
			if len(parts) > 1 {
				if p := strings.TrimSpace(parts[1]); p != "" {
					resolved.Commit = p
				}
			}
			if len(parts) > 2 {
				if p := strings.TrimSpace(parts[2]); p != "" {
					resolved.BuildDate = p
				}
			}
		}
	})
	return resolved
}

// String returns a compact "version (commit)" representation.
func (i Info) String() string {
	if i.Commit != "" && i.Commit != "none" {
		return i.Version + " (" + i.Commit + ")"
	}
	return i.Version
}
