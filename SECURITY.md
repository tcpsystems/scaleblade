# Security policy

## Scope

This repository is an unofficial client for a third-party portal. It does not
run a service and stores no account data. Security issues in scope include:

- leaking session cookies or other credentials through output, errors, files,
  subprocesses, or network requests to another host;
- bypassing the read-only design (`scaleblade request` accepting methods other
  than GET, MCP read-only tools mutating state, dangerous tools running without
  both gates);
- path validation or redirect handling that could send the session cookie to an
  unintended origin;
- unsafe file handling for the cookie file;
- dependency, build or CI supply-chain issues.

Out of scope: vulnerabilities in Scaleblade's own portal, and account actions
performed by a user with their own credentials.

## Reporting a vulnerability

Please use GitHub's private vulnerability reporting on
`https://github.com/tcpsystems/scaleblade` (Security → Report a vulnerability).
If that is unavailable, open an issue that describes the class of problem
**without any credential, account data, or exploit detail**, and ask for a
private channel.

Do not include:

- cookie names or values,
- session identifiers, actor ids, emails, project ids or labels,
- raw captures, HAR files or screenshots from a real account.

Expect an acknowledgement when the maintainer sees it. There is no bounty and no
formal SLA.

## Credential handling

The client authenticates with browser session cookies. They are equivalent to a
password for the portal session.

- The default cookie file is `~/.config/scaleblade/cookies.json` (or
  `$XDG_CONFIG_HOME/scaleblade/cookies.json`). On Unix the client rejects files
  with group/world permission bits and requires `0600`.
- `SCALEBLADE_COOKIE` exists for advanced use but may expose the value in process
  listings or shell history. Prefer `--cookie-file` or
  `SCALEBLADE_COOKIE_FILE`.
- Cookie values are attached only to requests to the configured portal origin
  (default `https://my.scaleblade.com`). Cross-host redirects are refused.
- The client does not log requests and does not print the cookie header.
  Transport errors are passed through a redaction step.
- The cookie file is ignored by `.gitignore`; the test suite uses synthetic
  headers and never touches the real file.

If a cookie value is exposed, assume the session is compromised. Sign out of the
portal (or let the session expire) and sign in again. Rotating the browser
session invalidates the copied value.

## Dangerous operations

`vm power`, `server power` and the MCP power/reinstall tools mutate
infrastructure.

- The CLI requires `--yes`.
- The MCP server lists the tools but refuses to execute them unless started with
  `--allow-dangerous`; even then each call must include `"confirm": true`.
- There are no automatic retries for mutations.
- No mutation was performed while building or testing this repository.

Do not enable `--allow-dangerous` for an agent that has broad task authority or
for a shared account.

## Data handling

The tools send API responses to the terminal or MCP client. They do not persist
responses. `--json` and the MCP tools return the server's `data` payload
verbatim; treat it as sensitive account data and avoid pasting it into issues or
shared logs.

## Supported versions

Only the latest commit on `main` is supported. This is a volunteer project built
on an undocumented API; there are no backports.
