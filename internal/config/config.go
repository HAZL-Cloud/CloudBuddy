// Package config holds CloudBuddy's runtime settings and secrets for one
// launch.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

const Version = "0.1.0-rc1"

type Config struct {
	Port      int
	DataDir   string // ~/.cloudbuddy (protected, D-027)
	DBURL     string
	Bin       string // absolute path of the cloudbuddy binary
	Home      string
	Dev       bool   // accept the Vite dev origin
	Epoch     string // changes every launch; approvals and releases are bound to it
	GateToken string // secret for hook / exec / gateway requests (can't approve anything)
}

func Random(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func Load() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, err
	}
	c := Config{Port: 7420, Home: home, Epoch: Random(8), GateToken: Random(24)}
	c.DataDir = os.Getenv("CLOUDBUDDY_HOME")
	if c.DataDir == "" {
		c.DataDir = filepath.Join(home, ".cloudbuddy")
	}
	c.DBURL = os.Getenv("CLOUDBUDDY_DB")
	if c.DBURL == "" {
		c.DBURL = "postgres://cloudbuddy:cloudbuddy@127.0.0.1:54329/cloudbuddy?sslmode=disable"
	}
	if p := os.Getenv("CLOUDBUDDY_PORT"); p != "" {
		fmt.Sscanf(p, "%d", &c.Port)
	}
	exe, err := os.Executable()
	if err != nil {
		return c, err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	c.Bin = exe
	if err := os.MkdirAll(filepath.Join(c.DataDir, "runs"), 0o700); err != nil {
		return c, err
	}
	if r, err := filepath.EvalSymlinks(c.DataDir); err == nil {
		c.DataDir = r
	}
	return c, nil
}

func (c Config) GateURL() string { return fmt.Sprintf("http://127.0.0.1:%d", c.Port) }

// RunDir is where per-run files (MCP specs, settings) live.
func (c Config) RunDir(runID string) string { return filepath.Join(c.DataDir, "runs", runID) }
