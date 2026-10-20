package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tcpsystems/scaleblade/internal/api"
)

func (a *App) cmdAuth(ctx context.Context, g globals, args []string) error {
	pos, err := parseCommand(args, nil, nil)
	if err != nil {
		return err
	}
	sub := "me"
	if len(pos) > 0 {
		sub, pos = pos[0], pos[1:]
	}
	if len(pos) != 0 || (sub != "me" && sub != "check") {
		return usagef("usage: scaleblade auth me | auth check [--json]")
	}
	c, err := a.client(g)
	if err != nil {
		if sub == "check" {
			return a.reportAuthFailure(g, err)
		}
		return err
	}
	raw, err := c.GetRaw(ctx, "/v1/auth/me", "")
	if err != nil {
		if sub == "check" {
			return a.reportAuthFailure(g, err)
		}
		return err
	}
	if g.jsonOut {
		if sub == "check" {
			var me api.Me
			if err := decodeData(raw, &me); err != nil {
				return err
			}
			return a.printJSON(authStatus(me, c, true))
		}
		return a.printRaw(raw)
	}
	var me api.Me
	if err := decodeData(raw, &me); err != nil {
		return err
	}
	if sub == "check" {
		pairs := [][2]string{
			{"Status", "ok"},
			{"Role", dash(me.Role)},
			{"Session expires", dash(me.ExpiresAt)},
			{"Remaining", remainingTime(me.ExpiresAt)},
			{"Credentials", c.CredentialSource()},
		}
		return writeKV(a.Out, pairs)
	}
	return writeKV(a.Out, [][2]string{
		{"Role", dash(me.Role)},
		{"Actor", dash(me.ActorID)},
		{"Effective actor", fmt.Sprintf("%s (%s)", dash(me.EffectiveActorEmail), dash(me.EffectiveActorID))},
		{"Impersonating", yesNo(me.Impersonating)},
		{"Session expires", dash(me.ExpiresAt)},
		{"Credentials", c.CredentialSource()},
	})
}

// authStatus is the machine-readable shape of `auth check`. It contains no
// cookie values, only their source description.
func authStatus(me api.Me, c *api.Client, ok bool) map[string]any {
	st := map[string]any{
		"authenticated": ok,
		"role":          me.Role,
		"expires_at":    me.ExpiresAt,
		"remaining":     remainingTime(me.ExpiresAt),
	}
	if c != nil {
		st["credentials"] = c.CredentialSource()
	}
	return st
}

// reportAuthFailure classifies a failed session probe. HTTP 401 and any
// credential-resolution error mean a human must refresh the cookie file; other
// failures stay generic so a network outage is not mistaken for an expired
// session.
func (a *App) reportAuthFailure(g globals, err error) error {
	var apiErr *api.APIError
	expired := errors.Is(err, api.ErrNoCredentials) ||
		(errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized)
	if !expired {
		return err
	}
	if g.jsonOut {
		st := map[string]any{"authenticated": false, "reason": err.Error()}
		if werr := a.printJSON(st); werr != nil {
			return werr
		}
	}
	return authError{err: err}
}

// remainingTime renders the gap to an RFC 3339 timestamp, or "-" when the
// timestamp is missing or unparseable.
func remainingTime(ts string) string {
	t, err := parseAPITime(ts)
	if err != nil {
		return "-"
	}
	d := time.Until(t)
	if d <= 0 {
		return "expired"
	}
	return d.Round(time.Minute).String()
}

func (a *App) cmdProjects(ctx context.Context, g globals, args []string) error {
	pos, err := parseCommand(args, nil, nil)
	if err != nil {
		return err
	}
	sub := "list"
	if len(pos) > 0 {
		sub, pos = pos[0], pos[1:]
	}
	c, err := a.client(g)
	if err != nil {
		return err
	}
	switch sub {
	case "list":
		if len(pos) != 0 {
			return usagef("usage: scaleblade projects list")
		}
		raw, err := c.GetRaw(ctx, "/v1/portal/projects", "")
		if err != nil {
			return err
		}
		if g.jsonOut {
			return a.printRaw(raw)
		}
		var projects []api.Project
		if err := decodeData(raw, &projects); err != nil {
			return err
		}
		rows := make([][]string, 0, len(projects))
		for _, p := range projects {
			rows = append(rows, []string{p.ID, dash(p.Label), archived(p.ArchivedAt), dash(p.UpdatedAt)})
		}
		return writeTable(a.Out, []string{"ID", "LABEL", "ARCHIVED", "UPDATED"}, rows)
	case "get":
		if len(pos) != 1 {
			return usagef("usage: scaleblade projects get <project-id>")
		}
		id := pos[0]
		raw, err := c.GetRaw(ctx, "/v1/portal/projects/"+url.PathEscape(id), id)
		if err != nil {
			return err
		}
		if g.jsonOut {
			return a.printRaw(raw)
		}
		var p api.Project
		if err := decodeData(raw, &p); err != nil {
			return err
		}
		if err := writeKV(a.Out, [][2]string{
			{"ID", dash(p.ID)},
			{"Label", dash(p.Label)},
			{"Owner account", dash(p.OwnerAccountID)},
			{"Created", dash(p.CreatedAt)},
			{"Updated", dash(p.UpdatedAt)},
			{"Archived", archived(p.ArchivedAt)},
		}); err != nil {
			return err
		}
		if len(p.MemberViews) > 0 {
			fmt.Fprintln(a.Out)
			rows := make([][]string, 0, len(p.MemberViews))
			for _, m := range p.MemberViews {
				rows = append(rows, []string{dash(m.Email), dash(m.DisplayName), dash(m.Role), dash(m.AccountID)})
			}
			return writeTable(a.Out, []string{"EMAIL", "NAME", "ROLE", "ACCOUNT"}, rows)
		}
		return nil
	default:
		return usagef("unknown projects subcommand %q", sub)
	}
}

func (a *App) cmdCompute(ctx context.Context, g globals, args []string) error {
	var kind string
	pos, err := parseCommand(args, map[string]*string{"kind": &kind}, nil)
	if err != nil {
		return err
	}
	sub := "list"
	if len(pos) > 0 {
		sub, pos = pos[0], pos[1:]
	}
	project, err := requireProject(g)
	if err != nil {
		return err
	}
	c, err := a.client(g)
	if err != nil {
		return err
	}
	switch sub {
	case "list":
		if len(pos) != 0 {
			return usagef("usage: scaleblade compute list --project <id>")
		}
		raw, err := c.GetRaw(ctx, "/v1/portal/compute", project)
		if err != nil {
			return err
		}
		if g.jsonOut {
			return a.printRaw(raw)
		}
		var servers []api.Server
		if err := decodeData(raw, &servers); err != nil {
			return err
		}
		rows := make([][]string, 0, len(servers))
		for _, s := range servers {
			rows = append(rows, []string{s.ID, dash(s.Label), dash(s.Hostname), dash(s.OperationalState), dash(s.PrimaryIPAddress)})
		}
		return writeTable(a.Out, []string{"ID", "LABEL", "HOSTNAME", "STATE", "PRIMARY IP"}, rows)
	case "get":
		if len(pos) != 1 {
			return usagef("usage: scaleblade compute get <server-id> --project <id>")
		}
		raw, err := c.GetRaw(ctx, "/v1/portal/compute/"+url.PathEscape(pos[0]), project)
		if err != nil {
			return err
		}
		if g.jsonOut {
			return a.printRaw(raw)
		}
		var s api.Server
		if err := decodeData(raw, &s); err != nil {
			return err
		}
		return writeKV(a.Out, [][2]string{
			{"ID", dash(s.ID)},
			{"Label", dash(s.Label)},
			{"Hostname", dash(s.Hostname)},
			{"State", dash(s.OperationalState)},
			{"Billing state", dash(s.BillingState)},
			{"Primary IP", dash(s.PrimaryIPAddress)},
			{"Location", dash(s.LocationLabel)},
			{"Project", dash(s.ProjectID)},
			{"Created", dash(s.CreatedAt)},
			{"Archived", archived(s.ArchivedAt)},
		})
	case "profiles":
		if len(pos) != 0 {
			return usagef("usage: scaleblade compute profiles [--kind operating_system] --project <id>")
		}
		if kind == "" {
			kind = "operating_system"
		}
		raw, err := c.GetRaw(ctx, "/v1/portal/compute/os-profiles?kind="+url.QueryEscape(kind), project)
		if err != nil {
			return err
		}
		if g.jsonOut {
			return a.printRaw(raw)
		}
		var profiles []api.OSProfile
		if err := decodeData(raw, &profiles); err != nil {
			return err
		}
		rows := make([][]string, 0, len(profiles))
		for _, p := range profiles {
			rows = append(rows, []string{p.ID, dash(p.Family), dash(p.Version), dash(p.Kind), dash(p.Architecture)})
		}
		return writeTable(a.Out, []string{"ID", "FAMILY", "VERSION", "KIND", "ARCH"}, rows)
	default:
		return usagef("unknown compute subcommand %q", sub)
	}
}

func (a *App) cmdVMs(ctx context.Context, g globals, args []string) error {
	var metricRange string
	pos, err := parseCommand(args, map[string]*string{"range": &metricRange}, nil)
	if err != nil {
		return err
	}
	sub := "list"
	if len(pos) > 0 {
		sub, pos = pos[0], pos[1:]
	}
	project, err := requireProject(g)
	if err != nil {
		return err
	}
	c, err := a.client(g)
	if err != nil {
		return err
	}
	switch sub {
	case "list":
		if len(pos) != 0 {
			return usagef("usage: scaleblade vms list --project <id>")
		}
		raw, err := c.GetRaw(ctx, "/v1/portal/virtualization/virtual-machines", project)
		if err != nil {
			return err
		}
		if g.jsonOut {
			return a.printRaw(raw)
		}
		var vms []api.VM
		if err := decodeData(raw, &vms); err != nil {
			return err
		}
		rows := make([][]string, 0, len(vms))
		for _, v := range vms {
			rows = append(rows, []string{
				v.ID, dash(v.Label), dash(v.Hostname), dash(v.OperationalState),
				strconv.FormatInt(v.VCPU, 10), strconv.FormatInt(v.MemoryMB, 10),
				strconv.FormatInt(v.StorageGB, 10), dash(v.PrimaryIPAddress),
			})
		}
		return writeTable(a.Out, []string{"ID", "LABEL", "HOSTNAME", "STATE", "VCPU", "MEM MB", "DISK GB", "PRIMARY IP"}, rows)
	case "get":
		if len(pos) != 1 {
			return usagef("usage: scaleblade vms get <vm-id> --project <id>")
		}
		raw, err := c.GetRaw(ctx, "/v1/portal/virtualization/virtual-machines/"+url.PathEscape(pos[0]), project)
		if err != nil {
			return err
		}
		if g.jsonOut {
			return a.printRaw(raw)
		}
		var v api.VM
		if err := decodeData(raw, &v); err != nil {
			return err
		}
		return writeVM(a, v)
	case "metrics":
		if len(pos) != 1 {
			return usagef("usage: scaleblade vms metrics <vm-id> [--range 1h|24h|7d] --project <id>")
		}
		if metricRange == "" {
			metricRange = "1h"
		}
		if !validRange(metricRange) {
			return usagef("invalid --range %q (examples: 1h, 24h, 7d)", metricRange)
		}
		raw, err := c.GetRaw(ctx, "/v1/portal/virtualization/virtual-machines/"+url.PathEscape(pos[0])+"/metrics?range="+url.QueryEscape(metricRange), project)
		if err != nil {
			return err
		}
		if g.jsonOut {
			return a.printRaw(raw)
		}
		var metrics []api.Metric
		if err := decodeData(raw, &metrics); err != nil {
			return err
		}
		fmt.Fprintf(a.Out, "range: %s  samples: %d\n", metricRange, len(metrics))
		if len(metrics) == 0 {
			return nil
		}
		last := metrics[len(metrics)-1]
		return writeTable(a.Out, []string{"TIME", "CPU %", "MEM BYTES", "MEM MAX", "IN BPS", "OUT BPS"}, [][]string{{
			formatSampleTime(last.Time),
			fmt.Sprintf("%.2f", last.CPUPercent),
			strconv.FormatInt(last.MemoryBytes, 10),
			strconv.FormatInt(last.MemoryMaxBytes, 10),
			fmt.Sprintf("%.0f", last.InboundBPS),
			fmt.Sprintf("%.0f", last.OutboundBPS),
		}})
	case "deployment":
		if len(pos) != 1 {
			return usagef("usage: scaleblade vms deployment <vm-id> --project <id>")
		}
		raw, err := c.GetRaw(ctx, "/v1/portal/virtualization/virtual-machines/"+url.PathEscape(pos[0])+"/deployment", project)
		if err != nil {
			return err
		}
		if g.jsonOut {
			return a.printRaw(raw)
		}
		var d api.Deployment
		if err := decodeData(raw, &d); err != nil {
			return err
		}
		if err := writeVM(a, d.VirtualMachine); err != nil {
			return err
		}
		fmt.Fprintln(a.Out)
		return writeKV(a.Out, [][2]string{
			{"Task", dash(d.Task.DisplayID)},
			{"Task state", dash(d.Task.State)},
			{"Progress", fmt.Sprintf("%.0f%%", d.Task.Progress)},
			{"Task type", dash(d.Task.Type)},
		})
	default:
		return usagef("unknown vms subcommand %q", sub)
	}
}

func (a *App) cmdVM(ctx context.Context, g globals, args []string) error {
	pos, err := parseCommand(args, nil, nil)
	if err != nil {
		return err
	}
	if len(pos) != 3 || pos[0] != "power" {
		return usagef("usage: scaleblade vm power <vm-id> <action> --project <id> --yes")
	}
	return a.power(ctx, g, pos[1], pos[2], "/v1/portal/virtualization/virtual-machines/", "VM")
}

func (a *App) cmdServer(ctx context.Context, g globals, args []string) error {
	pos, err := parseCommand(args, nil, nil)
	if err != nil {
		return err
	}
	if len(pos) != 3 || pos[0] != "power" {
		return usagef("usage: scaleblade server power <server-id> <action> --project <id> --yes")
	}
	return a.power(ctx, g, pos[1], pos[2], "/v1/portal/compute/", "server")
}

var actionPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

func (a *App) power(ctx context.Context, g globals, id, action, prefix, noun string) error {
	project, err := requireProject(g)
	if err != nil {
		return err
	}
	if !actionPattern.MatchString(action) {
		return usagef("invalid action %q", action)
	}
	if !g.yes {
		return usagef("refusing to POST a %s power action without --yes", noun)
	}
	c, err := a.client(g)
	if err != nil {
		return err
	}
	path := prefix + url.PathEscape(id) + "/power/" + url.PathEscape(action)
	resp, err := c.PostJSON(ctx, path, project, map[string]string{"idempotency_key": newRequestID()})
	if err != nil {
		return err
	}
	if g.jsonOut {
		return a.printRaw(resp.Data)
	}
	fmt.Fprintf(a.Out, "%s power action %q accepted (HTTP %d)\n", noun, action, resp.Status)
	if len(resp.Data) > 0 && string(resp.Data) != "null" {
		return a.printRaw(resp.Data)
	}
	return nil
}

func (a *App) cmdNetwork(ctx context.Context, g globals, args []string) error {
	var limit, offset string
	pos, err := parseCommand(args, map[string]*string{"limit": &limit, "offset": &offset}, nil)
	if err != nil {
		return err
	}
	sub := "list"
	if len(pos) > 0 {
		sub, pos = pos[0], pos[1:]
	}
	project, err := requireProject(g)
	if err != nil {
		return err
	}
	c, err := a.client(g)
	if err != nil {
		return err
	}
	switch sub {
	case "list":
		if len(pos) != 0 {
			return usagef("usage: scaleblade network [list] [--limit N] [--offset N] --project <id>")
		}
		q := url.Values{}
		if limit != "" {
			if err := positiveInt("limit", limit); err != nil {
				return err
			}
			q.Set("limit", limit)
		}
		if offset != "" {
			if _, err := strconv.ParseInt(offset, 10, 64); err != nil {
				return usagef("invalid --offset %q", offset)
			}
			q.Set("offset", offset)
		}
		path := "/v1/portal/network"
		if len(q) > 0 {
			path += "?" + q.Encode()
		}
		raw, err := c.GetRaw(ctx, path, project)
		if err != nil {
			return err
		}
		if g.jsonOut {
			return a.printRaw(raw)
		}
		var page api.AllocationPage
		if err := decodeData(raw, &page); err != nil {
			return err
		}
		rows := make([][]string, 0, len(page.Items))
		for _, al := range page.Items {
			service := strings.Trim(strings.Join([]string{al.ServiceType, al.ServiceID}, "/"), "/")
			rows = append(rows, []string{al.ID, dash(al.Address), dash(al.Kind), dash(al.State), dash(service)})
		}
		if err := writeTable(a.Out, []string{"ID", "ADDRESS", "KIND", "STATE", "SERVICE"}, rows); err != nil {
			return err
		}
		fmt.Fprintf(a.Out, "total: %d  offset: %d  limit: %d\n", page.Total, page.Offset, page.Limit)
		return nil
	case "pricing":
		if len(pos) != 0 {
			return usagef("usage: scaleblade network pricing --project <id>")
		}
		raw, err := c.GetRaw(ctx, "/v1/portal/network/pricing", project)
		if err != nil {
			return err
		}
		if g.jsonOut {
			return a.printRaw(raw)
		}
		var p api.Pricing
		if err := decodeData(raw, &p); err != nil {
			return err
		}
		if err := writeKV(a.Out, [][2]string{
			{"Configured", yesNo(p.Configured)},
			{"Updated", dash(p.UpdatedAt)},
		}); err != nil {
			return err
		}
		if len(p.FloatingIPv4) > 0 {
			fmt.Fprintln(a.Out)
			rows := make([][]string, 0, len(p.FloatingIPv4))
			for _, tier := range p.FloatingIPv4 {
				enabled, price := "-", "-"
				if tier.Enabled != nil {
					enabled = yesNo(*tier.Enabled)
				}
				if tier.PricePence != nil {
					price = pence(*tier.PricePence)
				}
				rows = append(rows, []string{enabled, price})
			}
			return writeTable(a.Out, []string{"FLOATING IPV4 ENABLED", "PRICE"}, rows)
		}
		return nil
	default:
		return usagef("unknown network subcommand %q", sub)
	}
}

func (a *App) cmdBilling(ctx context.Context, g globals, args []string) error {
	pos, err := parseCommand(args, nil, nil)
	if err != nil {
		return err
	}
	sub := "summary"
	if len(pos) > 0 {
		sub, pos = pos[0], pos[1:]
	}
	c, err := a.client(g)
	if err != nil {
		return err
	}
	switch sub {
	case "summary":
		if len(pos) != 0 {
			return usagef("usage: scaleblade billing")
		}
		raw, err := c.GetRaw(ctx, "/v1/portal/billing", g.project)
		if err != nil {
			return err
		}
		if g.jsonOut {
			return a.printRaw(raw)
		}
		var b api.BillingSummary
		if err := decodeData(raw, &b); err != nil {
			return err
		}
		fmt.Fprintf(a.Out, "credit balance: %s\n\n", pence(b.CreditBalancePence))
		if err := writeInvoices(a, b.Invoices); err != nil {
			return err
		}
		if len(b.Subscriptions) > 0 {
			fmt.Fprintln(a.Out)
			rows := make([][]string, 0, len(b.Subscriptions))
			for _, s := range b.Subscriptions {
				rows = append(rows, []string{
					dash(s.ServiceType), dash(s.ServiceID), dash(s.Period),
					pence(s.CurrentNetPricePence), dash(s.State), archived(s.ArchivedAt),
				})
			}
			return writeTable(a.Out, []string{"SERVICE", "ID", "PERIOD", "PRICE", "STATE", "ARCHIVED"}, rows)
		}
		return nil
	case "invoices":
		if len(pos) != 0 {
			return usagef("usage: scaleblade billing invoices")
		}
		raw, err := c.GetRaw(ctx, "/v1/portal/billing/invoices", g.project)
		if err != nil {
			return err
		}
		if g.jsonOut {
			return a.printRaw(raw)
		}
		var invoices []api.Invoice
		if err := decodeData(raw, &invoices); err != nil {
			return err
		}
		return writeInvoices(a, invoices)
	default:
		return usagef("unknown billing subcommand %q", sub)
	}
}

func (a *App) cmdTickets(ctx context.Context, g globals, args []string) error {
	pos, err := parseCommand(args, nil, nil)
	if err != nil {
		return err
	}
	sub := "list"
	if len(pos) > 0 {
		sub, pos = pos[0], pos[1:]
	}
	c, err := a.client(g)
	if err != nil {
		return err
	}
	switch sub {
	case "list":
		if len(pos) != 0 {
			return usagef("usage: scaleblade tickets [list]")
		}
		raw, err := c.GetRaw(ctx, "/v1/portal/support/tickets", g.project)
		if err != nil {
			return err
		}
		if g.jsonOut {
			return a.printRaw(raw)
		}
		var tickets []api.Ticket
		if err := decodeData(raw, &tickets); err != nil {
			return err
		}
		rows := make([][]string, 0, len(tickets))
		for _, t := range tickets {
			id := t.DisplayID
			if id == "" {
				id = t.ID
			}
			subject := t.Subject
			if subject == "" {
				subject = t.Title
			}
			state := t.State
			if state == "" {
				state = t.Status
			}
			rows = append(rows, []string{id, dash(state), dash(subject), dash(t.UpdatedAt)})
		}
		return writeTable(a.Out, []string{"ID", "STATE", "SUBJECT", "UPDATED"}, rows)
	case "departments":
		raw, err := c.GetRaw(ctx, "/v1/portal/support/departments", g.project)
		if err != nil {
			return err
		}
		if g.jsonOut {
			return a.printRaw(raw)
		}
		var departments []api.Department
		if err := decodeData(raw, &departments); err != nil {
			return err
		}
		rows := make([][]string, 0, len(departments))
		for _, d := range departments {
			rows = append(rows, []string{d.ID, dash(d.Key), dash(d.Label), yesNo(d.CustomerSelectable), yesNo(d.Protected)})
		}
		return writeTable(a.Out, []string{"ID", "KEY", "LABEL", "SELECTABLE", "PROTECTED"}, rows)
	case "statuses":
		raw, err := c.GetRaw(ctx, "/v1/portal/support/statuses", g.project)
		if err != nil {
			return err
		}
		if g.jsonOut {
			return a.printRaw(raw)
		}
		var statuses []api.TicketStatus
		if err := decodeData(raw, &statuses); err != nil {
			return err
		}
		rows := make([][]string, 0, len(statuses))
		for _, s := range statuses {
			rows = append(rows, []string{s.ID, dash(s.Key), dash(s.Label), dash(s.Color), dash(s.Semantic)})
		}
		return writeTable(a.Out, []string{"ID", "KEY", "LABEL", "COLOR", "SEMANTIC"}, rows)
	case "eligibility":
		raw, err := c.GetRaw(ctx, "/v1/portal/support/tickets/incident-eligibility", g.project)
		if err != nil {
			return err
		}
		if g.jsonOut {
			return a.printRaw(raw)
		}
		var e api.IncidentEligibility
		if err := decodeData(raw, &e); err != nil {
			return err
		}
		return writeKV(a.Out, [][2]string{
			{"Eligible", yesNo(e.Eligible)},
			{"Reason", dash(e.Reason)},
			{"Minimum MRR", pence(e.MinimumMRRPence)},
		})
	default:
		return usagef("unknown tickets subcommand %q", sub)
	}
}

func (a *App) cmdTasks(ctx context.Context, g globals, args []string) error {
	var stuckFor string
	pos, err := parseCommand(args, map[string]*string{"stuck": &stuckFor}, nil)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return usagef("usage: scaleblade tasks [--stuck 20m]")
	}
	var threshold time.Duration
	if stuckFor != "" {
		d, parseErr := time.ParseDuration(stuckFor)
		if parseErr != nil || d <= 0 {
			return usagef("invalid --stuck %q (use a duration such as 20m or 1h)", stuckFor)
		}
		threshold = d
	}
	c, err := a.client(g)
	if err != nil {
		return err
	}
	raw, err := c.GetRaw(ctx, "/v1/portal/tasks", g.project)
	if err != nil {
		return err
	}
	if g.jsonOut {
		if err := a.printRaw(raw); err != nil {
			return err
		}
		if threshold > 0 {
			var tasks []api.Task
			if err := decodeData(raw, &tasks); err != nil {
				return err
			}
			if stalled := stalledTasks(tasks, threshold); len(stalled) > 0 {
				return stalledError{msg: stalledSummary(stalled, threshold)}
			}
		}
		return nil
	}
	var tasks []api.Task
	if err := decodeData(raw, &tasks); err != nil {
		return err
	}
	rows := make([][]string, 0, len(tasks))
	for _, t := range tasks {
		resource := strings.Trim(strings.Join([]string{t.ResourceType, t.ResourceID}, "/"), "/")
		rows = append(rows, []string{
			dash(t.DisplayID), dash(t.Type), dash(t.State),
			fmt.Sprintf("%.0f%%", t.Progress), dash(resource), dash(t.UpdatedAt),
		})
	}
	if err := writeTable(a.Out, []string{"ID", "TYPE", "STATE", "PROGRESS", "RESOURCE", "UPDATED"}, rows); err != nil {
		return err
	}
	if threshold == 0 {
		return nil
	}
	stalled := stalledTasks(tasks, threshold)
	if len(stalled) == 0 {
		fmt.Fprintf(a.Out, "\nno task has been in flight for more than %s\n", threshold)
		return nil
	}
	fmt.Fprintf(a.Out, "\nno progress for more than %s:\n", threshold)
	detail := make([][]string, 0, len(stalled))
	for _, t := range stalled {
		detail = append(detail, []string{
			dash(t.DisplayID), dash(t.Type), dash(t.State),
			fmt.Sprintf("%.0f%%", t.Progress), dash(t.UpdatedAt), stalledFor(t),
		})
	}
	if err := writeTable(a.Out, []string{"ID", "TYPE", "STATE", "PROGRESS", "UPDATED", "IDLE"}, detail); err != nil {
		return err
	}
	return stalledError{msg: stalledSummary(stalled, threshold)}
}

// terminalTaskStates are the states in which a provider task cannot progress
// any further. Any other state counts as in flight, so an unrecognised state
// is reported rather than silently ignored.
var terminalTaskStates = map[string]bool{
	"succeeded":  true,
	"success":    true,
	"completed":  true,
	"complete":   true,
	"failed":     true,
	"error":      true,
	"cancelled":  true,
	"canceled":   true,
	"timed_out":  true,
	"timedout":   true,
	"expired":    true,
	"archived":   true,
	"superseded": true,
}

// stalledTasks returns in-flight tasks whose updated_at is older than the
// threshold. Tasks without a parseable timestamp are left out: guessing would
// produce false alarms.
func stalledTasks(tasks []api.Task, threshold time.Duration) []api.Task {
	now := time.Now()
	var out []api.Task
	for _, t := range tasks {
		if terminalTaskStates[strings.ToLower(strings.TrimSpace(t.State))] {
			continue
		}
		ts, err := parseAPITime(t.UpdatedAt)
		if err != nil {
			continue
		}
		if now.Sub(ts) > threshold {
			out = append(out, t)
		}
	}
	return out
}

func stalledFor(t api.Task) string {
	ts, err := parseAPITime(t.UpdatedAt)
	if err != nil {
		return "-"
	}
	return time.Since(ts).Round(time.Minute).String()
}

func stalledSummary(stalled []api.Task, threshold time.Duration) string {
	// Report the longest idle: the earliest updated_at among the stalled tasks.
	oldest, parseErr := parseAPITime(stalled[0].UpdatedAt)
	for _, t := range stalled[1:] {
		if ts, err := parseAPITime(t.UpdatedAt); err == nil && ts.Before(oldest) {
			oldest = ts
		}
	}
	msg := fmt.Sprintf("%d provider task(s) made no progress for more than %s", len(stalled), threshold)
	if parseErr == nil {
		msg += fmt.Sprintf("; longest idle %s", time.Since(oldest).Round(time.Minute))
	}
	return msg
}

// parseAPITime accepts the RFC 3339 variants the portal emits, with or without
// fractional seconds.
func parseAPITime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, errors.New("empty timestamp")
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.999999999Z0700"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse timestamp %q", s)
}

func (a *App) cmdRequest(ctx context.Context, g globals, args []string) error {
	pos, err := parseCommand(args, nil, nil)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usagef("usage: scaleblade request <GET /v1/path>")
	}
	if err := api.ValidatePath(pos[0]); err != nil {
		return usagef("%v", err)
	}
	c, err := a.client(g)
	if err != nil {
		return err
	}
	resp, err := c.Get(ctx, pos[0], g.project)
	if err != nil {
		return err
	}
	if g.rawOut {
		return a.printRaw(resp.Raw)
	}
	return a.printRaw(resp.Data)
}

func writeVM(a *App, v api.VM) error {
	return writeKV(a.Out, [][2]string{
		{"ID", dash(v.ID)},
		{"Label", dash(v.Label)},
		{"Hostname", dash(v.Hostname)},
		{"State", dash(v.OperationalState)},
		{"Billing state", dash(v.BillingState)},
		{"Billing term", dash(v.BillingTerm)},
		{"vCPU", strconv.FormatInt(v.VCPU, 10)},
		{"Memory MB", strconv.FormatInt(v.MemoryMB, 10)},
		{"Storage GB", strconv.FormatInt(v.StorageGB, 10)},
		{"Primary IP", dash(v.PrimaryIPAddress)},
		{"OS", dash(v.OSLabel)},
		{"Location", dash(v.LocationLabel)},
		{"Recurring price", pence(v.RecurringPricePence)},
		{"Renews", dash(v.RenewsAt)},
		{"Created", dash(v.CreatedAt)},
	})
}

func writeInvoices(a *App, invoices []api.Invoice) error {
	rows := make([][]string, 0, len(invoices))
	for _, inv := range invoices {
		id := inv.DisplayID
		if id == "" {
			id = inv.ID
		}
		rows = append(rows, []string{id, dash(inv.State), pence(inv.GrossPence), dash(inv.Currency), dash(inv.IssuedAt)})
	}
	return writeTable(a.Out, []string{"INVOICE", "STATE", "GROSS", "CUR", "ISSUED"}, rows)
}

func archived(v *string) string {
	if v == nil || *v == "" {
		return "no"
	}
	return "yes"
}

var rangePattern = regexp.MustCompile(`^[0-9]{1,3}[mhdw]$`)

func validRange(v string) bool { return rangePattern.MatchString(v) }

func positiveInt(name, v string) error {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return usagef("invalid --%s %q", name, v)
	}
	return nil
}

func formatSampleTime(v float64) string {
	if v <= 0 {
		return "-"
	}
	if v > 1e12 {
		return time.UnixMilli(int64(v)).UTC().Format(time.RFC3339)
	}
	return time.Unix(int64(v), 0).UTC().Format(time.RFC3339)
}
