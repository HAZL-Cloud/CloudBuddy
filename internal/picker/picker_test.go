package picker

import (
	"errors"
	"testing"
)

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"/Users/me/code/shop/\n": "/Users/me/code/shop",
		"/home/me/infra\n":       "/home/me/infra",
		"/\n":                    "/",
	} {
		if got, err := Clean(in); err != nil || got != want {
			t.Errorf("Clean(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := Clean("\n"); !errors.Is(err, ErrCancelled) {
		t.Errorf("empty answer: %v", err)
	}
	if _, err := Clean("code/shop"); err == nil {
		t.Errorf("relative path accepted")
	}
}
