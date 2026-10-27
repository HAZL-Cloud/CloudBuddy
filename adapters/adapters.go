// Package adapters embeds the built-in adapter files (D-012) so a single
// binary carries them.
package adapters

import (
	"embed"
	"io/fs"
)

//go:embed *.yaml
var files embed.FS

func FS() fs.FS { return files }
