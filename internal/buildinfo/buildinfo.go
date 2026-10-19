// Package buildinfo holds version metadata shared by the CLI and MCP server.
package buildinfo

// Version is the released tool version. Override at build time with:
//
//	-ldflags "-X github.com/tcpsystems/scaleblade/internal/buildinfo.Version=v1.2.3"
var Version = "0.1.0"
