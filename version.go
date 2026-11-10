// Package lazascrub holds the version number shared by the app, the
// command-line tool and the reports they write.
package lazascrub

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var version string

// Version returns the release number from the VERSION file.
func Version() string { return strings.TrimSpace(version) }
