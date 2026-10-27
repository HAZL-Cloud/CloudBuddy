//go:build embedweb

package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Built returns the embedded frontend (release builds: go build -tags embedweb).
func Built() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil
	}
	return sub
}
