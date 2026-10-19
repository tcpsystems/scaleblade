package cli

import (
	"context"
	"fmt"
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
	if len(pos) > 1 || (len(pos) == 1 && pos[0] != "me") {
		return usagef("usage: scaleblade auth me")
	}
	c, err := a.client(g)
	if err != nil {
		return err
	}
	raw, err := c.GetRaw(ctx, "/v1/auth/me", "")
	if err != nil {
		return err
	}
	if g.jsonOut {
		return a.printRaw(raw)
	}
	var me api.Me
	if err := decodeData(raw, &me); err != nil {
		return err
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
	pos, err := parseCommand(args, nil, nil)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return usagef("usage: scaleblade tasks")
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
		return a.printRaw(raw)
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
	return writeTable(a.Out, []string{"ID", "TYPE", "STATE", "PROGRESS", "RESOURCE", "UPDATED"}, rows)
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
