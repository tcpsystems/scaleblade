# scaleblade

**Unofficial, reverse-engineered CLI and MCP server for the Scaleblade provider
portal API.**

> **Disclaimer.** This project is not affiliated with, endorsed by, or supported
> by Scaleblade. The API it uses is undocumented and was reconstructed by
> observing the public web portal and its JavaScript bundles. It can change or
> break at any time. Using automation against the portal may violate its terms
> of service or acceptable-use policy. Use at your own risk, with your own
> account, and only where you are authorised. No warranty of any kind is
> provided; see [LICENSE](LICENSE) and [DISCLAIMER.md](DISCLAIMER.md).

## What it does

`scaleblade` is a small, dependency-free Go client for the portal at
`https://my.scaleblade.com`. It reuses the browser session cookies that the
portal itself uses; there is no separate API key or token flow.

- `scaleblade` — read-only CLI for sessions, projects, bare-metal compute,
  virtual machines, network allocations, billing, tickets and tasks, plus a
  guarded `vm power` / `server power` command.
- `scaleblade-mcp` — Model Context Protocol server over stdio, exposing the same
  read-only surface to agents. Mutating power/reinstall tools are disabled
  unless the server is started with `--allow-dangerous` **and** the call sets
  `confirm: true`.

The endpoint catalogue, observed response shapes, and what is verified versus
inferred are documented in [docs/api.md](docs/api.md). The reverse-engineering
method and its limits are in [docs/notes.md](docs/notes.md).

## Requirements

- Go 1.22 or newer to build.
- A Scaleblade account with access to the portal.
- A browser in which you are signed in to `https://my.scaleblade.com`.

The built binaries have no runtime dependencies.

## Build and install

```bash
git clone https://github.com/tcpsystems/scaleblade.git
cd scaleblade
make build            # ./bin/scaleblade and ./bin/scaleblade-mcp
# or:
go build -o bin/scaleblade ./cmd/scaleblade
go build -o bin/scaleblade-mcp ./cmd/scaleblade-mcp
```

To install into `$GOBIN`:

```bash
go install github.com/tcpsystems/scaleblade/cmd/scaleblade@latest
go install github.com/tcpsystems/scaleblade/cmd/scaleblade-mcp@latest
```

## Authentication: browser cookies

The portal API authenticates with the same session cookies your browser sends.
After signing in, copy them from your browser's developer tools:

1. Open `https://my.scaleblade.com` and sign in.
2. Open DevTools → **Application** (Chrome/Edge) or **Storage** (Firefox) →
   **Cookies** → `https://my.scaleblade.com`.
3. There are two relevant cookies: one issued by Cloudflare's bot check and one
   portal session cookie. Copy each name and value.
4. Create `~/.config/scaleblade/cookies.json` with this shape:

```json
{
  "domain": "my.scaleblade.com",
  "cookies": {
    "<cookie-name-1>": "<cookie-value-1>",
    "<cookie-name-2>": "<cookie-value-2>"
  }
}
```

5. Restrict the file:

```bash
chmod 600 ~/.config/scaleblade/cookies.json
```

The client accepts these alternative sources, in precedence order:

| Source | Meaning |
| --- | --- |
| `--cookie-file PATH` | Explicit JSON file. |
| `SCALEBLADE_COOKIE_FILE` | Path to a JSON file. |
| `SCALEBLADE_COOKIE` | Literal `name=value; name2=value2` header, or a path to a JSON file. |
| default | `$XDG_CONFIG_HOME/scaleblade/cookies.json`, else `~/.config/scaleblade/cookies.json`. |

On Unix, a cookie file with any group or world permission bits is rejected. The
client never prints, logs, or serialises cookie values, and it refuses to follow
redirects to a different host. There is no logout command; use the portal if you
want to end the session.

> Treat these cookies exactly like a password. They grant full access to your
> portal session. Do not paste them into issues, chat, CI logs, screenshots, or
> shell history. For day-to-day use prefer the JSON file over
> `SCALEBLADE_COOKIE` so the value does not appear in your process list.

### Cookie expiry

Sessions expire. When the portal returns HTTP 401 with code `unauthorized`, sign
in again in the browser and refresh `cookies.json`. The tools report the API
status; they do not attempt to reuse or refresh credentials.

## CLI usage

```bash
# Session and projects
scaleblade auth me
scaleblade projects list
scaleblade projects get <project-id>

# Bare-metal compute (project-scoped)
scaleblade compute list --project <project-id>
scaleblade compute get <server-id> --project <project-id>
scaleblade compute profiles --project <project-id>

# Virtual machines
scaleblade vms list --project <project-id>
scaleblade vms get <vm-id> --project <project-id>
scaleblade vms metrics <vm-id> --range 24h --project <project-id>
scaleblade vms deployment <vm-id> --project <project-id>

# Network, billing, support, tasks
scaleblade network --project <project-id>
scaleblade network pricing --project <project-id>
scaleblade billing
scaleblade billing invoices
scaleblade tickets
scaleblade tasks

# One read-only GET against any /v1 path
scaleblade request /v1/portal/tasks --project <project-id>

# Raw JSON (data payload) for every command
scaleblade vms list --project <project-id> --json
```

Project ids come from `scaleblade projects list`. They are sent as the
`x-project-id` header; some endpoints require it and return
`400 project_required` without it. `SCALEBLADE_PROJECT` can supply a default.

### Dangerous commands

```bash
scaleblade vm power <vm-id> reboot --project <project-id> --yes
scaleblade server power <server-id> reboot --project <project-id> --yes
```

These send real POST requests with an idempotency key. They are the only
mutating commands in the CLI and require `--yes`. The action vocabulary is
inferred from the portal bundle; see [docs/api.md](docs/api.md). The MCP server
keeps this surface behind two independent gates.

### Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Success. |
| 1 | API, network, credential or decode error. |
| 2 | Usage error (bad flag, missing `--project`, missing `--yes`). |

## MCP server

`scaleblade-mcp` speaks JSON-RPC 2.0 over stdio (newline-delimited JSON; also
accepts `Content-Length` framing). It implements `initialize`, `ping`,
`tools/list`, and `tools/call`.

Example client configuration:

```json
{
  "mcpServers": {
    "scaleblade": {
      "command": "/absolute/path/to/scaleblade-mcp",
      "args": ["--project", "<project-id>"],
      "env": {
        "SCALEBLADE_COOKIE_FILE": "/home/you/.config/scaleblade/cookies.json"
      }
    }
  }
}
```

Read-only tools include `scaleblade_auth_me`, `scaleblade_projects_list`,
`scaleblade_compute_list`, `scaleblade_compute_get`,
`scaleblade_compute_profiles`, `scaleblade_vms_list`, `scaleblade_vms_get`,
`scaleblade_vm_metrics`, `scaleblade_vm_deployment`,
`scaleblade_network_list`, `scaleblade_network_pricing`,
`scaleblade_firewall_list`, `scaleblade_billing_summary`,
`scaleblade_billing_invoices`, `scaleblade_tickets_list`,
`scaleblade_tasks_list`, and the generic read-only `scaleblade_request`.

Dangerous tools — `scaleblade_vm_power`, `scaleblade_server_power`, and
`scaleblade_server_action` (inferred reinstall/power paths) — are listed but
refuse to run unless the server was started with `--allow-dangerous`:

```json
{
  "mcpServers": {
    "scaleblade-admin": {
      "command": "/absolute/path/to/scaleblade-mcp",
      "args": ["--project", "<project-id>", "--allow-dangerous"],
      "env": {
        "SCALEBLADE_COOKIE_FILE": "/home/you/.config/scaleblade/cookies.json"
      }
    }
  }
}
```

Even then, every call must include `"confirm": true`. Without that argument the
tool returns an error and performs no request. Do not enable this mode for
general-purpose agents.

## Safety model

- No request is logged. Errors include the HTTP status, API error code and
  server message, never the cookie header.
- Redirects to a different host are refused.
- Only `GET` requests reach `/v1/...` through `scaleblade request`; the method is
  not configurable there.
- No admin endpoint is called by the CLI or the read-only MCP tools. Admin routes
  are documented as gated; the observed account received `403 forbidden`.
- No order, billing, impersonation, console, reinstall or power action was
  performed while producing this repository.
- The test suite uses `httptest` and synthetic fixtures only. No account data,
  cookie value, screenshot or raw capture is committed.

## Development

```bash
make fmt        # gofmt -w .
make vet        # go vet ./...
make test       # go test -race -shuffle=on ./...
make build
```

CI runs `gofmt -l`, `go vet`, `go test -race -shuffle=on`, and `go build` on
every push and pull request.

Repository layout:

```
cmd/scaleblade/        CLI entry point
cmd/scaleblade-mcp/    MCP stdio entry point
internal/api/          HTTP client, cookie loading, envelopes, typed shapes
internal/cli/          command parsing, formatting, commands
internal/mcp/          JSON-RPC/MCP server and tool registry
docs/                  API catalogue, methodology, REA notes
```

## Documentation

- [docs/api.md](docs/api.md) — endpoint catalogue, observed vs inferred, shapes.
- [docs/notes.md](docs/notes.md) — methodology, Cloudflare, roles, rate limits,
  risks, provenance.
- [docs/rea.md](docs/rea.md) — REA browser-capture setup and its result.
- [SECURITY.md](SECURITY.md) — credential handling and vulnerability reporting.
- [DISCLAIMER.md](DISCLAIMER.md) — legal/usage disclaimer.

## License

MIT. See [LICENSE](LICENSE).
