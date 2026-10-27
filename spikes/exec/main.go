// Command exec is the Phase 0 stand-in for cloudbuddy-exec: the only program
// allowed to run outside Claude Code's sandbox. Here it just runs its arguments.
package main

import (
	"fmt"
	"os"
	"os/exec"
)

func main() {
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	fmt.Fprintln(os.Stderr, "[spike-exec] running outside the sandbox:", os.Args[1:])
	c := exec.Command("/bin/sh", "-c", os.Args[1])
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			os.Exit(ee.ExitCode())
		}
		os.Exit(1)
	}
}
