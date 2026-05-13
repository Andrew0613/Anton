package app

import (
	"fmt"
	"io"

	"github.com/Andrew0613/Anton/internal/adopt"
	"github.com/Andrew0613/Anton/internal/buildinfo"
	"github.com/Andrew0613/Anton/internal/contextcmd"
	"github.com/Andrew0613/Anton/internal/doctor"
	"github.com/Andrew0613/Anton/internal/entrypoint"
	"github.com/Andrew0613/Anton/internal/gates"
	"github.com/Andrew0613/Anton/internal/handoff"
	"github.com/Andrew0613/Anton/internal/history"
	"github.com/Andrew0613/Anton/internal/memory"
	"github.com/Andrew0613/Anton/internal/migrate"
	"github.com/Andrew0613/Anton/internal/preflight"
	"github.com/Andrew0613/Anton/internal/taskstate"
	"github.com/Andrew0613/Anton/internal/threads"
	"github.com/Andrew0613/Anton/internal/versioncmd"
	"github.com/Andrew0613/Anton/internal/workspace"
)

const (
	exitOK    = 0
	exitUsage = 2
)

func globalUsageText() string {
	return fmt.Sprintf(`Anton %s is a reusable harness CLI.

Usage:
  anton [--json] <command> [...]
  anton doctor [--json]
  anton context [--json|--explain]
  anton preflight --profile <investigation|implementation> [--json]
  anton task-state <init|pulse|check|env|service|freshness|sync-card|close|reopen|retarget|import> [--json]
  anton handoff <build|persist-results> [--json]
  anton threads <doctor|recent|insights|brief|recipe> [--json]
  anton adopt plan [--json]
  anton memory <status|update> [--json]
  anton history <show|sync> [--json]
  anton gates <list|check> [--json]
  anton entrypoint check [--json]
  anton workspace <inspect|check|refs> [--json]
  anton migrate <plan|readiness> [--json]
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
	args = normalizeGlobalJSON(args)

	switch args[0] {
	case "doctor":
		return doctor.Run(args[1:], stdout, stderr, environ)
	case "context":
		return contextcmd.Run(args[1:], stdout, stderr, environ)
	case "preflight":
		return preflight.Run(args[1:], stdout, stderr, environ)
	case "task-state":
		return taskstate.Run(args[1:], stdout, stderr, environ)
	case "handoff":
		return handoff.Run(args[1:], stdout, stderr, environ)
	case "threads":
		return threads.Run(args[1:], stdout, stderr, environ)
	case "adopt":
		return adopt.Run(args[1:], stdout, stderr, environ)
	case "memory":
		return memory.Run(args[1:], stdout, stderr, environ)
	case "history":
		return history.Run(args[1:], stdout, stderr, environ)
	case "gates":
		return gates.Run(args[1:], stdout, stderr, environ)
	case "entrypoint":
		return entrypoint.Run(args[1:], stdout, stderr, environ)
	case "workspace":
		return workspace.Run(args[1:], stdout, stderr, environ)
	case "migrate":
		return migrate.Run(args[1:], stdout, stderr, environ)
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

func normalizeGlobalJSON(args []string) []string {
	if len(args) <= 1 || args[0] != "--json" {
		return args
	}

	normalized := make([]string, 0, len(args))
	normalized = append(normalized, args[1:]...)
	for _, arg := range normalized {
		if arg == "--json" {
			return normalized
		}
	}
	return append(normalized, "--json")
}
