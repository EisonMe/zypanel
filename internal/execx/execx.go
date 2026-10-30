// Package execx runs external commands without a shell.
package execx

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

// Result is the outcome of a command execution.
type Result struct {
	Argv     []string `json:"argv"`
	ExitCode int      `json:"exit_code"`
	Stdout   string   `json:"stdout"`
	Stderr   string   `json:"stderr"`
	Err      string   `json:"err,omitempty"`
	Duration string   `json:"duration"`
}

// OK reports whether the command ran and exited with status 0.
func (r Result) OK() bool { return r.Err == "" && r.ExitCode == 0 }

// Run executes argv with a timeout. It never uses a shell.
func Run(argv []string, timeout time.Duration) Result {
	start := time.Now()
	res := Result{Argv: argv}

	if len(argv) == 0 {
		res.Err = "empty argv"
		res.Duration = time.Since(start).String()
		return res
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res.Stdout = stdout.String()
	res.Stderr = stderr.String()
	res.Duration = time.Since(start).String()

	if err != nil {
		var exitErr *exec.ExitError
		switch {
		case errors.As(err, &exitErr):
			res.ExitCode = exitErr.ExitCode()
		case errors.Is(err, context.DeadlineExceeded):
			res.ExitCode = 124
			res.Err = "timeout after " + timeout.String()
		case errors.Is(err, exec.ErrNotFound):
			res.ExitCode = 127
			res.Err = "executable not found: " + argv[0]
		default:
			res.ExitCode = 1
			res.Err = err.Error()
		}
	}
	return res
}

// LookPath reports whether the named binary is available.
func LookPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
