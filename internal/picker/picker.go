// Package picker opens the computer's own folder chooser, so people pick a
// folder instead of typing its path. The browser can't do this: a web page
// never learns a folder's full path.
package picker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	ErrUnavailable = errors.New("no folder chooser on this computer")
	ErrCancelled   = errors.New("no folder chosen")
)

// macScript reads its prompt and start folder from argv, so neither is ever
// part of the script text. "activate" brings the chooser in front of the
// browser.
const macScript = `on run argv
	activate
	set thePrompt to item 1 of argv
	set theStart to item 2 of argv
	if theStart is "" then
		return POSIX path of (choose folder with prompt thePrompt)
	end if
	return POSIX path of (choose folder with prompt thePrompt default location (POSIX file theStart))
end run`

// Folder shows a folder chooser that starts at start (when it is a folder)
// and returns the full path of the folder picked. It stops when ctx ends.
func Folder(ctx context.Context, prompt, start string) (string, error) {
	if fi, err := os.Stat(start); err != nil || !fi.IsDir() {
		start = ""
	}
	var cmd *exec.Cmd
	switch {
	case runtime.GOOS == "darwin":
		cmd = exec.CommandContext(ctx, "/usr/bin/osascript", "-e", macScript, prompt, start)
	case has("zenity"):
		args := []string{"--file-selection", "--directory", "--title=" + prompt}
		if start != "" {
			args = append(args, "--filename="+start+string(filepath.Separator))
		}
		cmd = exec.CommandContext(ctx, "zenity", args...)
	case has("kdialog"):
		if start == "" {
			start, _ = os.UserHomeDir()
		}
		cmd = exec.CommandContext(ctx, "kdialog", "--title", prompt, "--getexistingdirectory", start)
	default:
		return "", ErrUnavailable
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		// Every chooser exits with 1 on Cancel (osascript adds error -128).
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return "", ErrCancelled
		}
		return "", errors.New(strings.TrimSpace(stderr.String()))
	}
	return Clean(string(out))
}

// Clean turns a chooser's answer into a folder path: trimmed, without the
// trailing slash macOS adds.
func Clean(out string) (string, error) {
	p := strings.TrimSpace(out)
	if p == "" {
		return "", ErrCancelled
	}
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
	}
	if !filepath.IsAbs(p) {
		return "", errors.New("the chooser didn't return a full path")
	}
	return p, nil
}

func has(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}
