//go:build !embedweb

// Package web holds the built frontend for release builds. Development
// builds serve web/dist from disk instead.
package web

import "io/fs"

// Built returns the embedded frontend, or nil in development builds.
func Built() fs.FS { return nil }
