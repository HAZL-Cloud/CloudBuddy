package recognize

import "io/fs"

// Adapters holds the loaded adapter files (Phase 4). A nil *Adapters means
// generic recognition only.
type Adapters struct {
	shell []shellRule
	mcp   []mcpRule
}

type shellRule struct{}
type mcpRule struct{}

func (a *Adapters) matchShell(s segment) (Draft, bool) { return Draft{}, false }
func (a *Adapters) matchMCP(c Call) (Draft, bool)      { return Draft{}, false }

// LoadAdapters reads adapter files from dir (if it exists) and the built-in
// set. Implemented in Phase 4.
func LoadAdapters(dir string, builtin fs.FS) (*Adapters, error) { return &Adapters{}, nil }
