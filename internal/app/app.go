package app

import (
	"fmt"
	"io"

	"github.com/Andrew0613/Anton/internal/buildinfo"
	"github.com/Andrew0613/Anton/internal/doctor"
	"github.com/Andrew0613/Anton/internal/handoff"
	"github.com/Andrew0613/Anton/internal/taskstate"
	"github.com/Andrew0613/Anton/internal/threads"
	"github.com/Andrew0613/Anton/internal/versioncmd"
)

const (
	exitOK    = 0
	exitUsage = 2
)

func globalUsageText() string {
	return fmt.Sprintf(`Anton %s is a reusable harness CLI.

Usage:
  anton doctor [--json]
  anton task-state <init|pulse|check|close|reopen|retarget|import> [--json]
  anton handoff build [--json]
  anton threads <doctor|recent|insights|brief|recipe> [--json]
  anton version [--json]

Flags:
  --help
  --version
`, buildinfo.Version)
}

func Run(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	if len(args) == 0 {
		_, _ = io.WriteString(stderr, globalUsageText())
		return exitUsage
	}

	switch args[0] {
	case "doctor":
		return doctor.Run(args[1:], stdout, stderr, environ)
	case "task-state":
		return taskstate.Run(args[1:], stdout, stderr, environ)
	case "handoff":
		return handoff.Run(args[1:], stdout, stderr, environ)
	case "threads":
		return threads.Run(args[1:], stdout, stderr, environ)
	case "version", "--version", "-v":
		return versioncmd.Run(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		_, _ = io.WriteString(stdout, globalUsageText())
		return exitOK
	default:
		_, _ = fmt.Fprintf(stderr, "unknown command: %s\n\n%s", args[0], globalUsageText())
		return exitUsage
	}
}
