# Scaleblade portal API catalogue

Reverse-engineered catalogue of the API served by `https://my.scaleblade.com`.
It is not official documentation. The provider can change any of this without
notice.

- **Probed:** 2026-10-19, using one ordinary portal account (role `portal`),
  read-only `GET` requests only, across all four projects visible to that
  account.
- **Static source:** 19 Next.js client chunks (~3.8 MB) fetched from the portal.
- **Method:** see [notes.md](notes.md).

## Legend

| Label | Meaning |
| --- | --- |
| `observed 200` | Request was sent during the probe and returned HTTP 200. |
| `observed 400/401/403/404` | Request was sent and returned that status. |
| `inferred` | Path and method recovered from the portal JavaScript bundle, or implied by the UI; **not executed** during the probe. |
| `gated` | Admin surface. Not executed. The tested portal account received `403` on admin routes. |
| `not probed` | Known/expected but untested because it needs an id or parameters we did not have. |

Unless marked otherwise, a `GET` returning `observed 200` had a valid session.

## Transport and envelopes

- Origin: `https://my.scaleblade.com`
- API prefix: `/v1`
- JSON success envelope: `{"data": ...}`
- JSON error envelope: `{"error": {"code": "...", "message": "...", "request_id": "..."}}`
  (`request_id` appears on some errors, not all).
- Error codes seen: `unauthorized`, `forbidden`, `project_required`,
  `not_found`, `validation_failed`. More codes are likely.
- Some endpoints return pagination inside `data`, for example
  `{"items": [...], "total": n, "offset": n, "limit": n}` or
  `{"items": [...], "page": n, "page_size": n, "total_items": n, "total_pages": n}`.
- `GET /v1/auth/me` without credentials: `401 unauthorized`.
- `GET /v1/admin/projects` with a portal-role session: `403 forbidden`.

## Authentication, roles and project scoping

Authentication is the browser cookie pair set by the portal. There is no
documented token or API-key flow in the client bundle.

- Cloudflare fronts the portal. A browser that has passed the bot check carries
  a Cloudflare clearance cookie; the API expects it alongside the portal session
  cookie. Requests without the pair fail at Cloudflare or at the app.
- `GET /v1/auth/me` exposes `role`, `actor_id`, `effective_actor_id`,
  `effective_actor_email`, `impersonating`, `session_id`, `expires_at`.
- The tested account was `role: "portal"`. Admin routes returned
  `403 forbidden`; an admin/impersonated session is expected to reach them
  (inferred).
- `x-project-id: <project _id>` selects the project. Observed behaviour:
  - `GET /v1/portal/compute` and `GET /v1/portal/network` return
    `400 project_required` without it.
  - `GET /v1/portal/billing`, `/support/tickets`, and `/tasks` returned `200`
    without it (account-wide views).
  - The header is accepted on project-resource GETs such as
    `/v1/portal/virtualization/virtual-machines`.
- The project list comes from `GET /v1/portal/projects`.

## Auth

| Method | Path | Scope | State | Notes |
| --- | --- | --- | --- | --- |
| GET | `/v1/auth/me` | no project | observed 200 | Session, actor, effective actor, role, expiry. |
| POST | `/v1/auth/logout` | no project | inferred | Not called; using it would end the browser/API session. |
| POST | `/v1/auth/login/totp` | no project | inferred | TOTP step in the web login flow. |
| POST | `/v1/auth/verify-email` | no project | inferred | Email verification flow. |
| POST | `/v1/auth/verify-email/resend` | no project | inferred | Resend verification. |
| POST | `/v1/auth/password/{...}` | no project | inferred | Password change/reset route template in the bundle. |

## Projects

| Method | Path | Scope | State | Notes |
| --- | --- | --- | --- | --- |
| GET | `/v1/portal/projects` | none | observed 200 | Array of projects. |
| GET | `/v1/portal/projects/{id}` | `x-project-id` | observed 200 | Project detail; adds hydrated `member_views`. |
| GET | `/v1/portal/projects/{id}/members` | `x-project-id` | observed 404 | GET is not the member route. |
| POST | `/v1/portal/projects` | none | inferred | Create project. |
| POST | `/v1/portal/projects/{id}/members` | `x-project-id` | inferred | Add member (JSON body). |
| DELETE | `/v1/portal/projects/{id}/members/{accountId}` | `x-project-id` | inferred | Remove member. |
| DELETE | `/v1/portal/projects/{id}` | `x-project-id` | inferred | Delete project. |
| GET | `/v1/portal/projects/profile` | none | inferred | Bundle route. |
| GET | `/v1/admin/projects` | none | gated (observed 403) | Admin project list. |
| GET | `/v1/admin/projects/admin-list` | none | gated | Query: `limit`, `page`, `search`, `account_id`. |
| GET | `/v1/admin/projects/profiles/{id}` | none | gated | Admin project profile. |
| GET | `/v1/admin/projects/{id}` | none | gated | Admin project detail. |

Observed project object keys: `_id`, `label`, `owner_account_id`, `members`,
`ownership_transfer`, `created_at`, `updated_at`, `archived_at`. Detail adds
`member_views[]` with `account_id`, `display_name`, `email`, `role`.

## Compute (bare metal)

| Method | Path | Scope | State | Notes |
| --- | --- | --- | --- | --- |
| GET | `/v1/portal/compute` | `x-project-id` required | observed 200 | Array of bare-metal servers; empty for all four observed projects. |
| GET | `/v1/portal/compute/os-profiles?kind=operating_system` | `x-project-id` | observed 200 | 14 OS install profiles. |
| GET | `/v1/portal/compute/{id}` | `x-project-id` | not probed | No bare-metal server existed to query. |
| POST | `/v1/portal/compute/{id}/{action}` | `x-project-id` | inferred | Generic action route; UI calls `power/{action}`. |
| POST | `/v1/portal/compute/{id}/power/{action}` | `x-project-id` | inferred | Power action, JSON body with `idempotency_key`. |
| POST | `/v1/portal/compute/{id}/reinstall` | `x-project-id` | inferred | Reinstall path implied by the action handler and `reinstall_task_id`; verify before use. |
| POST | `/v1/portal/compute/{id}/install-credentials/reveal` | `x-project-id` | inferred | Reveals install credentials; sensitive. |
| POST | `/v1/portal/compute/{id}/console-sessions` | `x-project-id` | inferred | Creates a console session. |
| GET | `/v1/portal/compute/available?{params}` | `x-project-id` | inferred | Available offers (query built by the UI). |
| POST | `/v1/portal/compute/{id}/quote` | `x-project-id` | inferred | Quote for a billing period. |
| POST | `/v1/portal/compute/{id}/order` | `x-project-id` | inferred | Order/renew a server. |
| PATCH | `/v1/portal/compute/{id}/customer-label` | `x-project-id` | inferred | Customer label. |
| GET | `/v1/portal/compute/{id}/billing` | `x-project-id` | gated/inferred | Admin billing view. |
| GET | `/v1/portal/compute/{id}/details` | `x-project-id` | gated/inferred | Admin details. |
| GET | `/v1/portal/compute/{id}/notes` | `x-project-id` | gated/inferred | Admin notes. |

OS profile keys observed: `_id`, `family`, `version`, `kind`, `architecture`,
`input_schema`, `installation` (`build`, `layouts`, `lifecycle`,
`login_username`, `notes`, `recommended`).

### Bandwidth (bundle-derived)

| Method | Path | Scope | State |
| --- | --- | --- | --- |
| GET | `/v1/portal/bandwidth/chart?{params}` (also `&per_server=true`) | project | inferred |
| GET | `/v1/portal/bandwidth/summary?{params}` | project | observed 400 without params |
| GET | `/v1/portal/bandwidth/resources/bare_metal/{id}` | project | inferred |

## Virtualization

| Method | Path | Scope | State | Notes |
| --- | --- | --- | --- | --- |
| GET | `/v1/portal/virtualization` | project | observed 404 | Not a route. |
| GET | `/v1/portal/virtualization/virtual-machines` | `x-project-id` | observed 200 | Array of VMs (0–1 in the observed projects). |
| GET | `/v1/portal/virtualization/virtual-machines/{id}` | `x-project-id` | observed 200 | VM detail. |
| GET | `/v1/portal/virtualization/virtual-machines/{id}/metrics?range=1h` | `x-project-id` | observed 200 | 70 samples for 1h/24h/7d. |
| GET | `/v1/portal/virtualization/virtual-machines/{id}/deployment` | `x-project-id` | observed 200 | `{virtual_machine, task}`. |
| GET | `/v1/portal/virtualization/images` | `x-project-id` | observed 200 | 6 images. |
| GET | `/v1/portal/virtualization/locations` | `x-project-id` | observed 200 | 2 locations. |
| GET | `/v1/portal/virtualization/datacentres` | `x-project-id` | observed 200 | 3 datacentres. |
| GET | `/v1/portal/virtualization/categories` | `x-project-id` | observed 200 | 2 categories. |
| GET | `/v1/portal/virtualization/catalogue?plan_id=...` | `x-project-id` | observed 400 | Validation error without query parameters. |
| POST | `/v1/portal/virtualization/virtual-machines/{id}/power/{action}` | `x-project-id` | inferred | JSON body with `idempotency_key`. |
| POST | `/v1/portal/virtualization/virtual-machines/{id}/password-reset` | `x-project-id` | inferred | Password reset. |
| POST | `/v1/portal/virtualization/virtual-machines/{id}/console-sessions` | `x-project-id` | inferred | Console/VNC session. |
| POST | `/v1/portal/virtualization/virtual-machines/{id}/credentials/reveal` | `x-project-id` | inferred | Reveal credentials; sensitive. |
| PATCH | `/v1/portal/virtualization/virtual-machines/{id}/label` | `x-project-id` | inferred | Rename. |
| GET | `/v1/portal/virtualization/orders` | project | observed 404 | GET is not supported; order flow is POST (inferred). |
| GET | `/v1/portal/virtualization/quotes` | project | observed 404 | Quote flow is POST (inferred). |
| POST | `/v1/portal/virtualization/orders` | project | inferred | Create VM order. |
| POST | `/v1/portal/virtualization/quotes` | project | inferred | Quote. |

VM keys observed: `_id`, `label`, `hostname`, `operational_state`,
`billing_state`, `billing_term`, `vcpu`, `memory_mb`, `storage_gb`,
`transfer_tb`, `transfer_unlimited`, `hourly_price_micropence`,
`recurring_price_pence`, `created_at`, `reinstall_task_id`,
`primary_ip_address`, `os_label`, `location_label`, `renews_at`.

Metric sample keys: `time`, `cpu_percent`, `memory_bytes`, `memory_max_bytes`,
`inbound_bps`, `outbound_bps`.

## Network

| Method | Path | Scope | State | Notes |
| --- | --- | --- | --- | --- |
| GET | `/v1/portal/network` | `x-project-id` required | observed 200 | `{items,total,offset,limit}`. |
| GET | `/v1/portal/network/pricing` | `x-project-id` | observed 200 | Floating IPv4 / IPv6 prefix pricing. |
| GET | `/v1/portal/network/floating-locations` | `x-project-id` | observed 200 | Empty for the observed project. |
| GET | `/v1/portal/network/floating-orders` | project | observed 404 | GET unsupported; POST flow inferred. |
| GET | `/v1/portal/network/ipv6-orders` | project | observed 404 | GET unsupported; POST flow inferred. |
| GET | `/v1/portal/network/subnets` | project | observed 404 | Portal GET route does not exist. |
| GET | `/v1/portal/network/limits` | project | observed 404 | Not a portal route (admin exists). |
| GET | `/v1/portal/network/profiles` | project | observed 404 | Not a portal route. |
| GET | `/v1/portal/network/filter-types` | project | observed 404 | Not a portal route. |
| GET | `/v1/portal/network/addresses` | project | observed 404 | Firewall addresses live under `/firewall/addresses`. |
| GET | `/v1/portal/network/assignments` | project | observed 404 | Not a portal GET route. |
| GET | `/v1/portal/network/allocations` | project | observed 404 | Allocation actions are POST subroutes. |
| POST | `/v1/portal/network/allocations/assign` | project | inferred | Assign an allocation. |
| POST | `/v1/portal/network/allocations/reserve` | project | inferred | Reserve an allocation. |
| POST | `/v1/portal/network/allocations/{id}/detach` | project | inferred | Detach. |
| POST | `/v1/portal/network/allocations/{id}/roas` | project | inferred | ROA flow. |
| POST | `/v1/portal/network/allocations/{id}/whois` | project | inferred | Whois lookup. |

Allocation keys observed: `_id`, `address`, `kind`, `state`, `project_id`,
`subnet_id`, `service_type`, `service_id`, `ownership_source`,
`ownership_started_at`, `ownership_ended_at`, `retained_service_type`,
`retained_service_id`, `order_idempotency_key`, `task_id`,
`reservation_reason`, `created_at`, `updated_at`, `archived_at`.

### Firewall, routing, colocation

| Method | Path | Scope | State | Notes |
| --- | --- | --- | --- | --- |
| GET | `/v1/portal/firewall` | project | observed 404 | Base is not a GET route. |
| GET | `/v1/portal/firewall/profiles` | `x-project-id` | observed 200 | Empty in the observed project. |
| GET | `/v1/portal/firewall/addresses` | `x-project-id` | observed 200 | Address rows with `eligible`, `protected_cidr`, `assignment`. |
| GET | `/v1/portal/firewall/attacks` | `x-project-id` | observed 200 | `{items,page,page_size,total_items,total_pages}`. |
| POST | `/v1/portal/firewall/profiles` | project | inferred | Create profile. |
| POST | `/v1/portal/firewall/addresses` | project | inferred | Create address/rule. |
| GET | `/v1/portal/routing/asns` | `x-project-id` | observed 200 | Empty. |
| GET | `/v1/portal/colocation/services` | `x-project-id` | observed 200 | Empty. |

## Billing

| Method | Path | Scope | State | Notes |
| --- | --- | --- | --- | --- |
| GET | `/v1/portal/billing` | project optional | observed 200 | Works without `x-project-id`. |
| GET | `/v1/portal/billing?include=service_labels` | project optional | inferred | Bundle uses this query for labels. |
| GET | `/v1/portal/billing/invoices` | project optional | observed 200 | Invoice array. |
| GET | `/v1/portal/billing/invoices/{id}` | project optional | inferred | Invoice detail. |
| GET | `/v1/portal/billing/invoices/{id}/pdf` | project optional | inferred | Binary PDF response. |
| GET | `/v1/portal/billing/credit/auto-reload` | project optional | observed 200 | `{enabled,threshold_pence,reload_amount_pence}`. |
| GET | `/v1/portal/billing/credit/topups` | project optional | observed 200 | Top-up records. |
| GET | `/v1/portal/billing/custom-services` | project optional | observed 200 | Empty. |
| GET | `/v1/portal/billing/custom-services/availability` | project optional | observed 200 | `{active}`. |
| GET | `/v1/portal/billing/payment-methods` | project optional | observed 200 | Empty. |
| GET | `/v1/portal/billing/legacy-invoices` | project optional | observed 200 | 6 legacy invoices. |
| GET | `/v1/portal/billing/tier-catalogue` | project optional | observed 404 | Not a portal route. |
| GET | `/v1/portal/billing/catalogue` | project optional | observed 404 | Not a portal route. |
| POST | `/v1/portal/billing/credit/topups` | project optional | inferred | Create top-up. |
| PATCH | `/v1/portal/billing/credit/auto-reload` | project optional | inferred | Update auto-reload. |
| POST | `/v1/portal/billing/payment-methods/setup` | project optional | inferred | Setup payment method. |
| GET | `/v1/portal/billing/payment-methods/setup/{id}` | project optional | inferred | Setup status. |
| DELETE | `/v1/portal/billing/payment-methods/{id}` | project optional | inferred | Remove payment method. |
| POST | `/v1/portal/orders` | project optional | inferred | Order creation. |
| POST | `/v1/portal/quotes` | project optional | inferred | Quote creation. |

Billing summary keys: `invoices`, `subscriptions`, `credit_balance_pence`.
Invoice keys observed: `_id`, `display_id`, `account_id`, `project_id`,
`state`, `invoice_type`, `currency`, `lines[]`, `net_pence`, `vat_pence`,
`gross_pence`, `credit_applied_pence`, `provider_paid_pence`,
`refunded_gross_pence`, `billing_snapshot`, `due_at`, `expires_at`, `issued_at`,
`paid_at`, `idempotency_key`, `aggregation_key`, `external_reference`,
`prepaid_consumed_by_subscription_id`, `recovery_hold`,
`auto_collection_paused`, `created_at`, `updated_at`, `archived_at`.
Subscription keys observed: `_id`, `account_id`, `project_id`, `service_id`,
`service_type`, `state`, `period`, `current_net_price_pence`,
`listed_net_price_pence`, `pricing_adjustment`, `prepaid_invoice_id`,
`cancellation_effective_at`, `restoration_task_id`, `renews_at`, `created_at`,
`updated_at`, `archived_at`.

## Support

| Method | Path | Scope | State | Notes |
| --- | --- | --- | --- | --- |
| GET | `/v1/portal/support/tickets` | project optional | observed 200 | Empty list; pagination is expected in `data.pagination`. |
| GET | `/v1/portal/support/tickets/{id}` | project optional | inferred | Ticket detail. |
| GET | `/v1/portal/support/tickets/{id}/replies` | project optional | inferred | Replies. |
| GET | `/v1/portal/support/tickets/{id}/status` | project optional | inferred | Status history. |
| GET | `/v1/portal/support/departments` | project optional | observed 200 | 4 departments. |
| GET | `/v1/portal/support/statuses` | project optional | observed 200 | 7 statuses. |
| GET | `/v1/portal/support/tickets/incident-eligibility` | project optional | observed 200 | `{eligible,reason,minimum_mrr_pence}`. |
| POST | `/v1/portal/support/tickets` | project optional | inferred | Create ticket. |
| POST | `/v1/portal/support/tickets/{id}/replies` | project optional | inferred | Reply. |
| PATCH | `/v1/portal/support/tickets/{id}/status` | project optional | inferred | Change status. |

## Tasks and notifications

| Method | Path | Scope | State | Notes |
| --- | --- | --- | --- | --- |
| GET | `/v1/portal/tasks` | project optional | observed 200 | 7 tasks; works without the project header. |
| GET | `/v1/portal/tasks?resource_type=...&resource_id=...` | project | inferred | Bundle filter used by compute/VM pages. |
| GET | `/v1/portal/tasks/{id}` | project optional | inferred | Task detail. |
| GET | `/v1/portal/notifications/preferences` | none | observed 200 | Email preferences. |
| PATCH | `/v1/portal/notifications/preferences` | none | inferred | Update preferences. |
| GET | `/v1/portal/notifications/notices/active` | none | observed 200 | Active notices (empty). |

Task keys observed: `_id`, `display_id`, `type`, `state`, `actor_id`,
`effective_actor_id`, `resource_type`, `resource_id`, `agent_id`,
`idempotency_key`, `correlation_id`, `operation_group`, `visibility`,
`parent_task_id`, `parent_step_id`, `retry_of_task_id`, `progress`, `steps[]`,
`created_at`, `updated_at`, `started_at`, `finished_at`, `waiting_reason`,
`resume_after`, `archived_at`. Step keys observed: `agent_id`, `attempt`,
`child_task_ids`, `executor`, `finished_at`, `id`, `key`, `result`,
`resume_after`, `sanitized_message`, `started_at`, `state`, `timeout_seconds`,
`waiting_reason`.

## Console endpoints

| Method | Path | Scope | State |
| --- | --- | --- | --- |
| GET/WS | `/v1/console/{id}` | token from console-session POST | inferred |
| GET/WS | `/v1/vm-console/{id}` | token from console-session POST | inferred |

## Admin surface (gated)

None of these were called beyond `GET /v1/admin/projects` (which returned
`403 forbidden` for the portal-role account). They are listed so the catalogue
is complete; a future admin-role implementation would need its own review.

| Domain | Bundle-derived paths (all `gated`) |
| --- | --- |
| Audit | `GET /v1/admin/audit/events` |
| Accounting | `/v1/admin/accounting/exports`, `/exports/{id}/retry`, `/reconcile`, `/reconciliations`, `/settings`, `/test` |
| Blackhole | `/v1/admin/blackhole/config`, `/events`, `/manual`, `/prefixes`, `/prefixes/{id}` |
| Billing | `/v1/admin/billing`, `/accounts/metrics`, `/analytics`, `/contracts`, `/contracts/{id}/document`, `/contracts/{id}/terminate`, `/credit/adjustments`, `/invoices/manual`, `/invoices/{id}/mark-paid`, `/invoices/{id}/pdf`, `/invoices/{id}/refund`, `/invoices/{id}/refund-context`, `/projects/{id}/services`, `/refunds`, `/refunds/{id}/complete-manual`, `/subscriptions/by-service/{id}` |
| Colocation | `/v1/admin/colocation/services` |
| Compute | `/v1/admin/compute`, `/catalogue-offers`, `/catalogue-offers/{id}`, `/{id}`, `/{id}/assign`, `/{id}/assignments/{id}`, `/{id}/billing`, `/{id}/boot-nic`, `/{id}/details`, `/{id}/notes`, query `project_id` |
| Firewall | `/v1/admin/firewall` |
| Identity | `/v1/admin/identity/accounts`, `/accounts/{id}`, `/impersonation/start`, `/impersonation/stop` |
| Infrastructure | `/v1/admin/infrastructure/*`: chassis, datacentres, locations, management-zones, network-devices, network-devices/{id}/port-control, pdus, port-control/resources/bare_metal/{id}, pxe-domains (+ `/{id}/agents`), pxe-networks, racks |
| Lifecycle | `/v1/admin/lifecycle/cancellations/{id}/retry`, `/cancellations?project_id=`, `/policies`, `/policies/{id}` |
| Network | `/v1/admin/network`, `/allocations/assign`, `/allocations/reserve`, `/allocations/{id}/attach`, `/allocations/{id}/unassign`, `/pricing`, `/services/bare_metal/{id}/available-primary-ips`, `/services/bare_metal/{id}/primary-ip`, `/subnets`, `/subnets/{id}`, `/subnets/{id}/addresses`, queries `family`, `project_id` |
| Notifications | `/v1/admin/notifications/notices`, `/notices/{id}/publish`, `/notices/{id}` |
| Projects | see Projects section |
| Routing | `/v1/admin/routing/asns`, `/prefixes/{id}`, `/routing/{id}` |
| Search | `/v1/admin/search` |
| Support | `/v1/admin/support/departments`, `/departments/{id}`, `/statuses`, `/statuses/{id}`, `/tickets`, `/tickets/{id}/messages/{id}`, `/tickets/{id}/notes` |
| Tasks | `/v1/admin/tasks`, `/tasks/{id}`, `/tasks/{id}/cancel`, `/tasks/{id}/retry` |
| Virtualization | `/v1/admin/virtualization/categories`, `/nodes`, `/nodes/{id}`, `/plans`, `/plans/{id}/pricing`, `/virtual-machines/{id}`, `/virtual-machines/{id}/migrate`, `/virtual-machines?project_id=` |

## Unknowns

- Rate limiting: no `x-ratelimit-*` or `Retry-After` headers and no `429` were
  observed, but the probe was small (~160 GETs). Assume a limit exists.
- Request/response bodies for every mutating route: the probe deliberately made
  no mutations. The shapes above are from GETs or from the client bundle.
- Exact action vocabulary for power/reinstall, and whether all actions share the
  same idempotency body.
- Behaviour of the account-wide endpoints when several projects are selected,
  and whether `x-project-id` is validated against the session's memberships.
- Admin role behaviour, impersonation flow, and the full billing/accounting
  surface.
