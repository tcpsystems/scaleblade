// Command scaleblade-mcp runs the unofficial Scaleblade MCP server over stdio.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tcpsystems/scaleblade/internal/api"
	"github.com/tcpsystems/scaleblade/internal/buildinfo"
	"github.com/tcpsystems/scaleblade/internal/mcp"
)

func main() {
	var (
		project        = flag.String("project", "", "default project id for x-project-id (env SCALEBLADE_PROJECT)")
		cookieFile     = flag.String("cookie-file", "", "cookie JSON file (env SCALEBLADE_COOKIE_FILE; default ~/.config/scaleblade/cookies.json)")
		baseURL        = flag.String("base-url", "", "portal origin (env SCALEBLADE_BASE_URL)")
		timeout        = flag.Duration("timeout", 30*time.Second, "per-request timeout")
		allowDangerous = flag.Bool("allow-dangerous", false, "enable mutating power/reinstall tools (they also require confirm=true)")
		version        = flag.Bool("version", false, "print the version and exit")
	)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "scaleblade-mcp %s - unofficial Scaleblade MCP server (stdio)\n\n", buildinfo.Version)
		fmt.Fprintln(flag.CommandLine.Output(), "Read-only tools are enabled by default. Power/reinstall tools require --allow-dangerous.")
		fmt.Fprintln(flag.CommandLine.Output(), "\nFlags:")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *version {
		fmt.Printf("scaleblade-mcp %s\n", buildinfo.Version)
		return
	}

	client, err := api.New(api.Config{
		BaseURL:    *baseURL,
		ProjectID:  *project,
		Timeout:    *timeout,
		CookieFile: *cookieFile,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "scaleblade-mcp: %v\n", err)
		os.Exit(1)
	}

	server, err := mcp.New(mcp.Options{
		Client:         client,
		Version:        buildinfo.Version,
		DefaultProject: client.ProjectID(),
		AllowDangerous: *allowDangerous,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "scaleblade-mcp: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := server.Run(ctx, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "scaleblade-mcp: %v\n", err)
		os.Exit(1)
	}
}
