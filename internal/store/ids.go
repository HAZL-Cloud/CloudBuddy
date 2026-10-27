package store

import (
	"crypto/rand"
	"encoding/hex"
)

// newID returns a random id with a short prefix ("st_3f9a…").
func newID(prefix string) string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

// NewID is exported for other packages that create records.
func NewID(prefix string) string { return newID(prefix) }
