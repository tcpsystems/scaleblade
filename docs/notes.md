# Reverse-engineering notes

This project exists because the provider publishes no API documentation. The
catalogue in [api.md](api.md) and the client in this repository were produced
from three evidence sources: the portal's own JavaScript bundles, authenticated
read-only HTTP requests, and a headless-browser runtime check.

**Date:** 2026-10-19. **Account:** one ordinary portal-role account. **Mutations:**
none.

## Evidence hierarchy

1. **Observed** — an HTTP request or a browser target was actually seen in this
   investigation.
2. **Derived** — a conclusion that follows from observed bundle text or observed
   responses (for example, a template's method inferred from surrounding code).
3. **Inferred / unknown** — plausible but not executed. Everything unexecuted is
   labelled `inferred` in [api.md](api.md), including admin endpoints, which were
   never called.

No credentials, account identifiers, labels, emails, IP addresses or raw
captures are stored in this repository. The test suite uses synthetic fixtures.

## Environment

| Component | Version / detail |
| --- | --- |
| Host | Fedora 44, x86_64 |
| Go | 1.27.2 local toolchain; module declares `go 1.22` |
| Browser | Chrome for Testing 155.0.8059.84 (`/home/kivox/.agent-browser/browsers/chrome-155.0.8059.84/chrome`) |
| Node (for scratch tooling only) | 22.23.1 |
| REA | 6.7.0 (`npx rea-agents@6.7.0`), CDP browser provider |

No runtime dependency on Node, Chrome or REA is needed to build or use this
repository; they were analysis tools.

## Step 1 — harvesting the Next.js client

The portal is a Next.js App Router application. Its client chunks contain the
API call sites, so the bundle is a de facto endpoint catalogue.

Procedure (generic):

1. Fetch the portal HTML with the authenticated browser session.
2. Collect `/_next/static/chunks/*.js` URLs from the HTML and the chunk graph.
3. Search the JavaScript for `/v1/`, `portal/`, `admin/`, `fetch(`, and template
   literals.
4. Normalise template interpolations (`${...}`) to `{param}` and resolve the
   scope variable that alternates between `admin` and `portal`.
5. Recover the HTTP method and body shape from the surrounding call site.

Observed during this pass:

- 19 chunks, ~3.8 MB total.
- 214 normalised `/v1/...` route snippets after de-duplication, including
  template routes and admin-only routes.
- The scope variable resolves to `admin` for admin pages and `portal` for
  customer pages; several pages are scope-agnostic
  (`/v1/${isAdmin ? "admin" : "portal"}/...`).
- Recurring route families: auth, projects, compute, virtualisation, network,
  firewall, routing, colocation, billing, support, tasks, notifications, and a
  large `/v1/admin/*` surface.
- Method evidence is uneven: `fetch(..., {method: "POST"})` is explicit for many
  mutations, but some templates are only reachable through a helper. Those are
  marked `inferred` in the catalogue.

The static pass is what surfaced routes such as
`/v1/portal/compute/{id}/install-credentials/reveal`,
`/v1/portal/virtualization/virtual-machines/{id}/deployment`, the firewall
sub-tabs (`profiles`, `addresses`, `attacks`), and the admin surface. It cannot
prove that a route exists, what it returns, or whether the current role may use
it.

## Step 2 — authenticated read-only probing

Credentials were loaded from a `0600` JSON file in memory. No cookie value was
written to a command line, a log, a fixture, or this repository during the
probe.

Rules used:

- `GET` only. No power, reinstall, order, billing, impersonation, console,
  password-reset, or admin calls.
- One project at a time via `x-project-id`, all four projects visible to the
  account.
- Response bodies were reduced to **schema shapes**: key names, JSON types, and
  array lengths. Values were discarded and never recorded.
- Requests were sequential and spaced modestly; no attempt was made to discover
  rate limits by hammering.

What was established (full detail in [api.md](api.md)):

- The API lives under `/v1`, same-origin, behind Cloudflare.
- Success envelope `{"data": ...}`; error envelope
  `{"error":{"code","message","request_id"?}}`.
- `GET /v1/auth/me` works with the cookie pair and returns role/actor/expiry.
- `GET /v1/portal/projects` returned four projects, none with members in the
  list view.
- The portal role receives `403 forbidden` on `/v1/admin/projects`.
- `x-project-id` is mandatory for compute and network (`400 project_required`)
  but not for billing, tickets or tasks.
- Some plausible routes are simply absent on the portal scope:
  `/v1/portal/virtualization`, `/v1/portal/firewall`, many `/v1/portal/network/*`
  subroutes, and `/v1/portal/billing/{tier-catalogue,catalogue}` returned
  `404 not_found`. Others live under different subpaths
  (`/firewall/profiles|addresses|attacks`, `/routing/asns`,
  `/colocation/services`).
- No bare-metal servers existed in any of the four projects, so compute detail,
  compute actions and reinstall remain inferred.
- One VM existed in one project; its detail, metrics
  (`range=1h|24h|7d`) and deployment endpoints were verified.
- Tickets were empty; the ticket detail/reply shapes remain inferred.
- `GET /v1/auth/me` without credentials returned `401 unauthorized`.

The probe also confirms that endpoint existence is project- and role-dependent.
Feature pages that exist for one project may 404 for another; the UI tolerates
this by hiding tabs. A client should surface `404 not_found` rather than infer a
transport failure.

## Step 3 — headless-browser runtime check

The credentials were injected into an isolated headless Chrome over CDP
(`Network.setCookies` against a loopback debugging port), then the page
`https://my.scaleblade.com/compute/bare-metal` was loaded. The authenticated
page loaded (title "Scaleblade Portal", redirected to the compute path). This
confirmed that the cookie pair is sufficient in a fresh browser profile and that
the origin/UA combination is accepted.

REA was asked to capture that page in connect mode. It failed with
`execution_failure` from provider `rea-playwright-browser-scenario`, both via the
MCP tool and the REA CLI. `rea doctor` reports a healthy Node/host but the
provider returned no root-cause detail. No raw capture was produced, and none is
committed. See [rea.md](rea.md) for the exact commands and the decision not to
retry indefinitely.

Consequence: runtime network capture is a gap in the evidence. The catalogue
rests on static bundle analysis plus read-only HTTP responses; the successful
authenticated page load is the only runtime observation.

## Cloudflare

- The portal is behind Cloudflare. The browser cookie set includes a Cloudflare
  clearance cookie, and the API expects it in addition to the portal session
  cookie.
- A missing or expired clearance cookie can produce an HTML challenge/403 rather
  than the JSON error envelope. The client distinguishes these: a non-JSON error
  becomes `http_error` with the HTTP status and **does not include the body**.
- User-Agent affinity is possible. The probe used a desktop Chrome UA matching
  the browser that created the session. The CLI uses a stable
  `scaleblade-cli` User-Agent; if Cloudflare rejects it, override the browser
  profile or obtain cookies with a matching client. This is a known fragility.
- Cloudflare may rate-limit or block by IP. Do not treat a 403 as proof of a
  permissions problem unless the body is the JSON error envelope.

## Roles and impersonation

- `GET /v1/auth/me` exposes both `actor_id` and `effective_actor_id`, plus
  `impersonating`. The effective actor is the identity used for authorisation.
- The observed account had `role: "portal"`. Admin endpoints returned
  `403 forbidden`, which is an application-level denial (JSON envelope), not a
  Cloudflare challenge.
- Impersonation is an admin feature (`/v1/admin/identity/impersonation/start` and
  `/stop` are in the bundle). This client does not expose it. If a session is
  impersonating, read tools operate as the effective actor; the CLI prints the
  flag so the operator is not surprised.
- Adding an admin mode would require a separate review of destructive and
  cross-tenant routes; the read-only client intentionally does not include one.

## Rate limits

No `x-ratelimit-*`, `RateLimit-*`, or `Retry-After` headers were observed, and no
`429` response occurred during roughly 160 read-only GETs. This does **not**
prove there is no limit. The portal is a shared service; clients should:

- keep request volume low and sequential for interactive use;
- back off on `429` and on 5xx;
- avoid polling tasks more often than necessary;
- never use this tool for scraping or bulk enumeration.

The client has a 30-second default timeout and no automatic retries.

## Idempotency and mutations

The portal's own mutation calls include an `idempotency_key` in the JSON body
(for example VM power and server power/actions). This client generates a random
key per command/tool call and sends the same body shape. It does not retry
mutations automatically. The exact server-side idempotency semantics were not
tested.

The action vocabulary (`start`, `shutdown`, `reboot`, `force-stop`, `reinstall`)
is inferred from UI strings and bundle handlers. Verify in the portal before
depending on it.

## Security handling

- Cookies are session credentials. They are never logged, serialised, or sent to
  any host other than the configured portal origin.
- The cookie file must be `0600`; group/world bits are rejected on Unix.
- HTTP redirects to a different host are refused.
- Non-JSON error bodies are not echoed, to avoid dumping challenge pages or
  unrelated data into logs.
- The generic MCP tool only permits `GET /v1/...`; method is not user-selectable.
- The repository and its tests contain no account data. `httptest` fixtures are
  synthetic.

## Provenance

| Artifact | Where it came from | Confidence |
| --- | --- | --- |
| Endpoint lists | Next.js chunks + live GETs | mixed; each row labelled |
| Response field names | Live read-only GETs reduced to shapes | observed |
| Admin paths | Next.js chunks | inferred/gated |
| Mutation bodies | Bundle call sites (`idempotency_key`) | inferred |
| Role behaviour | `/v1/auth/me` + one 403 | observed for this account |
| Rate limits | absence of headers/429 in a small probe | unknown |
| Runtime capture | headless page load succeeded; REA capture failed | partial |

If you re-run this work, date your observations, keep mutations out of the
discovery phase, and record status codes and shapes rather than values.
