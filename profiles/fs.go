// Package profiles holds embedded YAML style profiles.
package profiles

import "embed"

//go:embed *.yaml
var FS embed.FS
