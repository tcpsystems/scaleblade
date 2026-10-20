// Package cli implements the scaleblade command-line interface.
package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tcpsystems/scaleblade/internal/api"
)

// App holds the output streams for one CLI invocation.
type App struct {
	Out io.Writer
	Err io.Writer
	Ver string
}

// Run executes the CLI and returns a process exit code.
func Run(ctx context.Context, args []string, out, errOut io.Writer, version string) int {
	app := &App{Out: out, Err: errOut, Ver: version}
	return app.run(ctx, args)
}

type globals struct {
	project    string
	cookieFile string
	baseURL    string
	timeout    time.Duration
	jsonOut    bool
	rawOut     bool
	yes        bool
	help       bool
	version    bool
}

type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func usagef(format string, args ...any) error {
	return usageError{msg: fmt.Sprintf(format, args...)}
}

// Process exit codes. Codes 3 and 4 exist so cron jobs and the fleet harness
// can tell an expired browser session and a stalled provider task apart from a
// generic API or network failure.
const (
	exitGeneral         = 1
	exitUsage           = 2
	exitUnauthenticated = 3
	exitStalled         = 4
)

// authError wraps a failure that means the session must be refreshed by a
// human. It never carries cookie values.
type authError struct{ err error }

func (e authError) Error() string { return e.err.Error() }
func (e authError) Unwrap() error { return e.err }

// stalledError reports at least one provider task that has made no progress
// inside the caller's threshold.
type stalledError struct{ msg string }

func (e stalledError) Error() string { return e.msg }

func (a *App) run(ctx context.Context, args []string) int {
	g, rest, err := parseGlobals(args)
	if err != nil {
		return a.fail(err)
	}
	if g.version {
		fmt.Fprintf(a.Out, "scaleblade %s\n", a.Ver)
		return 0
	}
	if g.help || len(rest) == 0 {
		fmt.Fprint(a.Out, usageText)
		return 0
	}
	if rest[0] == "help" {
		fmt.Fprint(a.Out, usageText)
		return 0
	}
	if rest[0] == "version" {
		fmt.Fprintf(a.Out, "scaleblade %s\n", a.Ver)
		return 0
	}
	if err := a.dispatch(ctx, g, rest); err != nil {
		return a.fail(err)
	}
	return 0
}

func (a *App) fail(err error) int {
	var ue usageError
	if errors.As(err, &ue) {
		fmt.Fprintf(a.Err, "scaleblade: %v\n", err)
		fmt.Fprintln(a.Err, "run 'scaleblade help' for usage")
		return exitUsage
	}
	var se stalledError
	if errors.As(err, &se) {
		fmt.Fprintf(a.Err, "scaleblade: %v\n", err)
		return exitStalled
	}
	var ae authError
	var apiErr *api.APIError
	if errors.As(err, &ae) ||
		errors.Is(err, api.ErrNoCredentials) ||
		(errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized) {
		fmt.Fprintf(a.Err, "scaleblade: %v\n", err)
		fmt.Fprintln(a.Err, "session must be refreshed: sign in at https://my.scaleblade.com and rewrite the cookie file (see README.md)")
		return exitUnauthenticated
	}
	fmt.Fprintf(a.Err, "scaleblade: %v\n", err)
	return exitGeneral
}

func (a *App) dispatch(ctx context.Context, g globals, rest []string) error {
	cmd, args := rest[0], rest[1:]
	switch cmd {
	case "auth":
		return a.cmdAuth(ctx, g, args)
	case "projects":
		return a.cmdProjects(ctx, g, args)
	case "compute":
		return a.cmdCompute(ctx, g, args)
	case "vms":
		return a.cmdVMs(ctx, g, args)
	case "vm":
		return a.cmdVM(ctx, g, args)
	case "server":
		return a.cmdServer(ctx, g, args)
	case "network":
		return a.cmdNetwork(ctx, g, args)
	case "billing":
		return a.cmdBilling(ctx, g, args)
	case "tickets":
		return a.cmdTickets(ctx, g, args)
	case "tasks":
		return a.cmdTasks(ctx, g, args)
	case "request":
		return a.cmdRequest(ctx, g, args)
	default:
		return usagef("unknown command %q", cmd)
	}
}

func (a *App) client(g globals) (*api.Client, error) {
	c, err := api.New(api.Config{
		BaseURL:    g.baseURL,
		ProjectID:  g.project,
		Timeout:    g.timeout,
		CookieFile: g.cookieFile,
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

func requireProject(g globals) (string, error) {
	if strings.TrimSpace(g.project) == "" {
		return "", usagef("this command needs --project <project-id> (list ids with 'scaleblade projects list')")
	}
	return g.project, nil
}

// parseGlobals extracts global flags from anywhere in args. Unknown flags are
// left for the command parser.
func parseGlobals(args []string) (globals, []string, error) {
	var g globals
	var rest []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			rest = append(rest, args[i+1:]...)
			break
		}
		name, inline, hasInline, isFlag := splitFlag(arg)
		if !isFlag {
			rest = append(rest, arg)
			continue
		}
		value := func() (string, error) {
			if hasInline {
				return inline, nil
			}
			i++
			if i >= len(args) {
				return "", usagef("flag --%s needs a value", name)
			}
			return args[i], nil
		}
		switch name {
		case "json":
			v, err := boolValue(inline, hasInline, true)
			if err != nil {
				return g, nil, usagef("--json: %v", err)
			}
			g.jsonOut = v
		case "raw":
			v, err := boolValue(inline, hasInline, true)
			if err != nil {
				return g, nil, usagef("--raw: %v", err)
			}
			g.rawOut = v
		case "yes", "y":
			v, err := boolValue(inline, hasInline, true)
			if err != nil {
				return g, nil, usagef("--yes: %v", err)
			}
			g.yes = v
		case "help", "h":
			g.help = true
		case "version":
			g.version = true
		case "project", "p":
			v, err := value()
			if err != nil {
				return g, nil, err
			}
			g.project = v
		case "cookie-file":
			v, err := value()
			if err != nil {
				return g, nil, err
			}
			g.cookieFile = v
		case "base-url":
			v, err := value()
			if err != nil {
				return g, nil, err
			}
			g.baseURL = v
		case "timeout":
			v, err := value()
			if err != nil {
				return g, nil, err
			}
			d, parseErr := time.ParseDuration(v)
			if parseErr != nil || d <= 0 {
				return g, nil, usagef("invalid --timeout %q (use a duration such as 30s)", v)
			}
			g.timeout = d
		default:
			rest = append(rest, arg)
		}
	}
	return g, rest, nil
}

func splitFlag(arg string) (name, inline string, hasInline, isFlag bool) {
	if arg == "-h" {
		return "h", "", false, true
	}
	if arg == "-p" {
		return "p", "", false, true
	}
	if strings.HasPrefix(arg, "--") && len(arg) > 2 {
		body := arg[2:]
		if n, v, ok := strings.Cut(body, "="); ok {
			return n, v, true, true
		}
		return body, "", false, true
	}
	if strings.HasPrefix(arg, "-p") && len(arg) > 2 {
		body := arg[2:]
		if strings.HasPrefix(body, "=") {
			return "p", body[1:], true, true
		}
		return "p", body, true, true
	}
	return "", "", false, false
}

func boolValue(inline string, hasInline, fallback bool) (bool, error) {
	if !hasInline {
		return fallback, nil
	}
	v, err := strconv.ParseBool(inline)
	if err != nil {
		return false, fmt.Errorf("expected true or false, got %q", inline)
	}
	return v, nil
}

// parseCommand parses command-local flags. Values must follow the flag or use
// --flag=value. It returns positional arguments.
func parseCommand(args []string, strings map[string]*string, bools map[string]*bool) ([]string, error) {
	var pos []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		name, inline, hasInline, isFlag := splitFlag(arg)
		if !isFlag {
			pos = append(pos, arg)
			continue
		}
		if p, ok := strings[name]; ok {
			if hasInline {
				*p = inline
				continue
			}
			i++
			if i >= len(args) {
				return nil, usagef("flag --%s needs a value", name)
			}
			*p = args[i]
			continue
		}
		if p, ok := bools[name]; ok {
			v, err := boolValue(inline, hasInline, true)
			if err != nil {
				return nil, usagef("--%s: %v", name, err)
			}
			*p = v
			continue
		}
		return nil, usagef("unknown flag --%s", name)
	}
	return pos, nil
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
}

const usageText = `scaleblade - unofficial CLI for the Scaleblade provider portal API

This tool is not affiliated with Scaleblade. It talks to the portal's
reverse-engineered HTTP API with your browser session and may break at any time.
Read-only commands never mutate your account. 'vm power' and 'server power'
change infrastructure and require --yes.

Usage:
  scaleblade [global flags] <command> [subcommand] [args]

Commands:
  auth me                     Show the current session (GET /v1/auth/me)
  auth check                  Verify the session for scripts; exit 3 when stale
  projects list               List projects (GET /v1/portal/projects)
  projects get <id>           Show one project
  compute list                List bare-metal servers (needs --project)
  compute get <id>            Show one bare-metal server
  compute profiles            List OS install profiles
  vms list                    List virtual machines (needs --project)
  vms get <id>                Show one virtual machine
  vms metrics <id>            Show metric samples (--range 1h|24h|7d)
  vms deployment <id>         Show deployment state and task
  vm power <id> <action>      DANGEROUS: POST a VM power action (needs --yes)
  server power <id> <action>  DANGEROUS: POST a server power action (needs --yes)
  network [list]              List network allocations (needs --project)
  network pricing             Show network pricing
  billing                     Show invoices, subscriptions and credit
  billing invoices            List invoices
  tickets                     List support tickets
  tasks [--stuck 20m]         List tasks; exit 4 when a task has stopped moving
  request <GET /v1/path>      Perform one read-only GET request
  help                        Show this help
  version                     Print the version

Global flags:
  -p, --project ID            Value for the x-project-id header
                              (env SCALEBLADE_PROJECT)
      --json                  Print the raw JSON data payload
      --raw                   With 'request': print the full response envelope
      --cookie-file PATH      Cookie JSON file (env SCALEBLADE_COOKIE_FILE;
                              default ~/.config/scaleblade/cookies.json)
      --base-url URL          Override the portal origin
                              (env SCALEBLADE_BASE_URL)
      --timeout DURATION      Per-request timeout (env SCALEBLADE_TIMEOUT;
                              default 30s)
      --yes                   Confirm a mutating command
  -h, --help                  Show help
      --version               Print the version

Cookie extraction and the full endpoint catalog are documented in README.md
and docs/api.md. Never share your cookie values: they are session credentials.
`
