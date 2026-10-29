package shellenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseKeepsValuesWithEquals(t *testing.T) {
	env := parse([]string{"A=1", "B=x=y", "", "C="})
	if env["A"] != "1" || env["B"] != "x=y" || env["C"] != "" || len(env) != 3 {
		t.Fatalf("got %v", env)
	}
}

func TestLoginHasPathAndHome(t *testing.T) {
	env := Login()
	if env["PATH"] == "" || env["HOME"] == "" {
		t.Fatalf("no PATH or HOME: %d vars", len(env))
	}
	for k := range env {
		if len(k) > 7 && k[:7] == "DIRENV_" {
			t.Errorf("home folder direnv state leaked in: %s", k)
		}
	}
}

func TestDirenvWithoutEnvrcChangesNothing(t *testing.T) {
	env := map[string]string{"PATH": os.Getenv("PATH"), "HOME": os.Getenv("HOME"), "KEEP": "1"}
	if note := Direnv(env, t.TempDir()); note != "" {
		t.Errorf("note without .envrc: %s", note)
	}
	if env["KEEP"] != "1" {
		t.Errorf("env changed: %v", env)
	}
}

func TestDirenvBlockedEnvrcGivesNote(t *testing.T) {
	if _, err := os.Stat("/opt/homebrew/bin/direnv"); err != nil {
		t.Skip("direnv not installed")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".envrc"), []byte("export CB_TEST_DIRENV=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"PATH": os.Getenv("PATH"), "HOME": os.Getenv("HOME")}
	if note := Direnv(env, dir); note == "" {
		t.Errorf("no note for a blocked .envrc")
	}
	if env["CB_TEST_DIRENV"] != "" {
		t.Errorf("blocked .envrc was loaded")
	}
}
