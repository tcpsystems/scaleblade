package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/tcpsystems/scaleblade/internal/api"
)

// Handler executes one tool and returns JSON to display.
type Handler func(ctx context.Context, args map[string]json.RawMessage) (json.RawMessage, error)

// Tool is one MCP tool definition.
type Tool struct {
	Name        string
	Title       string
	Description string
	InputSchema map[string]any
	ReadOnly    bool
	Destructive bool
	Idempotent  bool
	Dangerous   bool
	Handler     Handler
}

var (
	actionPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	rangePattern  = regexp.MustCompile(`^[0-9]{1,3}[mhdw]$`)
)

func (s *Server) defineTools() []Tool {
	return []Tool{
		{
			Name:        "scaleblade_auth_me",
			Title:       "Current session",
			Description: "Return the authenticated portal session: actor, effective actor, role, impersonation state and expiry. Read-only.",
			InputSchema: object(nil),
			ReadOnly:    true,
			Idempotent:  true,
			Handler: func(ctx context.Context, _ map[string]json.RawMessage) (json.RawMessage, error) {
				return s.get(ctx, "/v1/auth/me", "")
			},
		},
		{
			Name:        "scaleblade_projects_list",
			Title:       "List projects",
			Description: "List projects visible to the session. Project _id values are used as the x-project-id value by project-scoped tools. Read-only.",
			InputSchema: object(nil),
			ReadOnly:    true,
			Idempotent:  true,
			Handler: func(ctx context.Context, _ map[string]json.RawMessage) (json.RawMessage, error) {
				return s.get(ctx, "/v1/portal/projects", "")
			},
		},
		{
			Name:        "scaleblade_compute_list",
			Title:       "List bare-metal servers",
			Description: "List bare-metal compute servers in a project. Requires x-project-id, which is supplied from the project argument or the server default. Read-only.",
			InputSchema: object(map[string]any{"project": stringProp("Project _id (x-project-id). Optional when the server was started with --project.")}),
			ReadOnly:    true,
			Idempotent:  true,
			Handler: func(ctx context.Context, args map[string]json.RawMessage) (json.RawMessage, error) {
				project, err := s.requireProject(args)
				if err != nil {
					return nil, err
				}
				return s.get(ctx, "/v1/portal/compute", project)
			},
		},
		{
			Name:        "scaleblade_compute_get",
			Title:       "Get bare-metal server",
			Description: "Get one bare-metal server by id. Read-only.",
			InputSchema: object(map[string]any{
				"project": stringProp("Project _id (x-project-id)."),
				"id":      stringProp("Server id."),
			}, "id"),
			ReadOnly:   true,
			Idempotent: true,
			Handler: func(ctx context.Context, args map[string]json.RawMessage) (json.RawMessage, error) {
				project, id, err := s.projectAndID(args)
				if err != nil {
					return nil, err
				}
				return s.get(ctx, "/v1/portal/compute/"+url.PathEscape(id), project)
			},
		},
		{
			Name:        "scaleblade_compute_profiles",
			Title:       "List OS install profiles",
			Description: "List operating-system install profiles for bare-metal ordering and reinstall. Read-only.",
			InputSchema: object(map[string]any{
				"project": stringProp("Project _id (x-project-id)."),
				"kind":    stringProp("Profile kind; defaults to operating_system."),
			}),
			ReadOnly:   true,
			Idempotent: true,
			Handler: func(ctx context.Context, args map[string]json.RawMessage) (json.RawMessage, error) {
				project, err := s.requireProject(args)
				if err != nil {
					return nil, err
				}
				kind := optionalString(args, "kind")
				if kind == "" {
					kind = "operating_system"
				}
				return s.get(ctx, "/v1/portal/compute/os-profiles?kind="+url.QueryEscape(kind), project)
			},
		},
		{
			Name:        "scaleblade_vms_list",
			Title:       "List virtual machines",
			Description: "List virtualization virtual machines in a project. Read-only.",
			InputSchema: object(map[string]any{"project": stringProp("Project _id (x-project-id).")}),
			ReadOnly:    true,
			Idempotent:  true,
			Handler: func(ctx context.Context, args map[string]json.RawMessage) (json.RawMessage, error) {
				project, err := s.requireProject(args)
				if err != nil {
					return nil, err
				}
				return s.get(ctx, "/v1/portal/virtualization/virtual-machines", project)
			},
		},
		{
			Name:        "scaleblade_vms_get",
			Title:       "Get virtual machine",
			Description: "Get one virtual machine by id. Read-only.",
			InputSchema: object(map[string]any{
				"project": stringProp("Project _id (x-project-id)."),
				"id":      stringProp("Virtual machine id."),
			}, "id"),
			ReadOnly:   true,
			Idempotent: true,
			Handler: func(ctx context.Context, args map[string]json.RawMessage) (json.RawMessage, error) {
				project, id, err := s.projectAndID(args)
				if err != nil {
					return nil, err
				}
				return s.get(ctx, "/v1/portal/virtualization/virtual-machines/"+url.PathEscape(id), project)
			},
		},
		{
			Name:        "scaleblade_vm_metrics",
			Title:       "Virtual machine metrics",
			Description: "Return metric samples (CPU, memory, network) for a virtual machine. range is a duration such as 1h, 24h or 7d. Read-only.",
			InputSchema: object(map[string]any{
				"project": stringProp("Project _id (x-project-id)."),
				"id":      stringProp("Virtual machine id."),
				"range":   stringProp("Sample window, for example 1h, 24h or 7d. Defaults to 1h."),
			}, "id"),
			ReadOnly:   true,
			Idempotent: true,
			Handler: func(ctx context.Context, args map[string]json.RawMessage) (json.RawMessage, error) {
				project, id, err := s.projectAndID(args)
				if err != nil {
					return nil, err
				}
				rng := optionalString(args, "range")
				if rng == "" {
					rng = "1h"
				}
				if !rangePattern.MatchString(rng) {
					return nil, fmt.Errorf("invalid range %q; use a value such as 1h, 24h or 7d", rng)
				}
				return s.get(ctx, "/v1/portal/virtualization/virtual-machines/"+url.PathEscape(id)+"/metrics?range="+url.QueryEscape(rng), project)
			},
		},
		{
			Name:        "scaleblade_vm_deployment",
			Title:       "Virtual machine deployment",
			Description: "Return the virtual machine's current deployment record and task. Read-only.",
			InputSchema: object(map[string]any{
				"project": stringProp("Project _id (x-project-id)."),
				"id":      stringProp("Virtual machine id."),
			}, "id"),
			ReadOnly:   true,
			Idempotent: true,
			Handler: func(ctx context.Context, args map[string]json.RawMessage) (json.RawMessage, error) {
				project, id, err := s.projectAndID(args)
				if err != nil {
					return nil, err
				}
				return s.get(ctx, "/v1/portal/virtualization/virtual-machines/"+url.PathEscape(id)+"/deployment", project)
			},
		},
		{
			Name:        "scaleblade_network_list",
			Title:       "List network allocations",
			Description: "List IP allocations for a project. Supports limit and offset. Read-only.",
			InputSchema: object(map[string]any{
				"project": stringProp("Project _id (x-project-id)."),
				"limit":   intProp("Maximum rows to return."),
				"offset":  intProp("Pagination offset."),
			}),
			ReadOnly:   true,
			Idempotent: true,
			Handler: func(ctx context.Context, args map[string]json.RawMessage) (json.RawMessage, error) {
				project, err := s.requireProject(args)
				if err != nil {
					return nil, err
				}
				q := url.Values{}
				if v, ok := intArg(args, "limit"); ok {
					q.Set("limit", strconv.FormatInt(v, 10))
				}
				if v, ok := intArg(args, "offset"); ok {
					q.Set("offset", strconv.FormatInt(v, 10))
				}
				path := "/v1/portal/network"
				if len(q) > 0 {
					path += "?" + q.Encode()
				}
				return s.get(ctx, path, project)
			},
		},
		{
			Name:        "scaleblade_network_pricing",
			Title:       "Network pricing",
			Description: "Return configured floating-IP and prefix pricing for a project. Read-only.",
			InputSchema: object(map[string]any{"project": stringProp("Project _id (x-project-id).")}),
			ReadOnly:    true,
			Idempotent:  true,
			Handler: func(ctx context.Context, args map[string]json.RawMessage) (json.RawMessage, error) {
				project, err := s.requireProject(args)
				if err != nil {
					return nil, err
				}
				return s.get(ctx, "/v1/portal/network/pricing", project)
			},
		},
		{
			Name:        "scaleblade_firewall_list",
			Title:       "Firewall profiles, addresses or attacks",
			Description: "List firewall data for a project. kind is profiles, addresses or attacks. Read-only.",
			InputSchema: object(map[string]any{
				"project": stringProp("Project _id (x-project-id)."),
				"kind":    enumProp("Firewall view.", "profiles", "addresses", "attacks"),
			}),
			ReadOnly:   true,
			Idempotent: true,
			Handler: func(ctx context.Context, args map[string]json.RawMessage) (json.RawMessage, error) {
				project, err := s.requireProject(args)
				if err != nil {
					return nil, err
				}
				kind := optionalString(args, "kind")
				switch kind {
				case "profiles", "addresses", "attacks":
				case "":
					kind = "profiles"
				default:
					return nil, fmt.Errorf("invalid kind %q; use profiles, addresses or attacks", kind)
				}
				return s.get(ctx, "/v1/portal/firewall/"+kind, project)
			},
		},
		{
			Name:        "scaleblade_billing_summary",
			Title:       "Billing summary",
			Description: "Return invoices, subscriptions and credit balance (pence). Account-wide; project is optional. Read-only.",
			InputSchema: object(map[string]any{"project": optionalProjectProp()}),
			ReadOnly:    true,
			Idempotent:  true,
			Handler: func(ctx context.Context, args map[string]json.RawMessage) (json.RawMessage, error) {
				return s.get(ctx, "/v1/portal/billing", optionalString(args, "project"))
			},
		},
		{
			Name:        "scaleblade_billing_invoices",
			Title:       "List invoices",
			Description: "List invoices visible to the session. Read-only.",
			InputSchema: object(map[string]any{"project": optionalProjectProp()}),
			ReadOnly:    true,
			Idempotent:  true,
			Handler: func(ctx context.Context, args map[string]json.RawMessage) (json.RawMessage, error) {
				return s.get(ctx, "/v1/portal/billing/invoices", optionalString(args, "project"))
			},
		},
		{
			Name:        "scaleblade_tickets_list",
			Title:       "List support tickets",
			Description: "List support tickets visible to the session. Read-only.",
			InputSchema: object(map[string]any{"project": optionalProjectProp()}),
			ReadOnly:    true,
			Idempotent:  true,
			Handler: func(ctx context.Context, args map[string]json.RawMessage) (json.RawMessage, error) {
				return s.get(ctx, "/v1/portal/support/tickets", optionalString(args, "project"))
			},
		},
		{
			Name:        "scaleblade_tasks_list",
			Title:       "List tasks",
			Description: "List asynchronous provider tasks (deployments, power operations and similar). Read-only.",
			InputSchema: object(map[string]any{"project": optionalProjectProp()}),
			ReadOnly:    true,
			Idempotent:  true,
			Handler: func(ctx context.Context, args map[string]json.RawMessage) (json.RawMessage, error) {
				return s.get(ctx, "/v1/portal/tasks", optionalString(args, "project"))
			},
		},
		{
			Name:        "scaleblade_request",
			Title:       "Raw read-only GET",
			Description: "Perform one GET request under /v1 and return the data payload. Only GET is possible, so this tool cannot mutate state. Prefer the specific tools above.",
			InputSchema: object(map[string]any{
				"path":    stringProp("Absolute API path beginning with /v1/ (query string allowed)."),
				"project": optionalProjectProp(),
			}, "path"),
			ReadOnly:   true,
			Idempotent: true,
			Handler: func(ctx context.Context, args map[string]json.RawMessage) (json.RawMessage, error) {
				path, err := stringArg(args, "path")
				if err != nil {
					return nil, err
				}
				if err := api.ValidatePath(path); err != nil {
					return nil, err
				}
				return s.get(ctx, path, optionalString(args, "project"))
			},
		},
		{
			Name:        "scaleblade_vm_power",
			Title:       "DANGEROUS: virtual machine power",
			Description: "DANGEROUS: POST a power action for a virtual machine (for example start, shutdown, reboot, force-stop). Changes real infrastructure. Requires server flag --allow-dangerous and confirm=true.",
			InputSchema: object(map[string]any{
				"project": stringProp("Project _id (x-project-id)."),
				"id":      stringProp("Virtual machine id."),
				"action":  stringProp("Power action, for example start, shutdown, reboot or force-stop."),
				"confirm": boolProp("Must be true. Confirms this mutating call."),
			}, "id", "action", "confirm"),
			ReadOnly:    false,
			Destructive: true,
			Dangerous:   true,
			Handler: func(ctx context.Context, args map[string]json.RawMessage) (json.RawMessage, error) {
				return s.power(ctx, args, "/v1/portal/virtualization/virtual-machines/")
			},
		},
		{
			Name:        "scaleblade_server_power",
			Title:       "DANGEROUS: bare-metal server power",
			Description: "DANGEROUS: POST a power action for a bare-metal server (for example start, shutdown, reboot, force-stop). Changes real infrastructure. Requires server flag --allow-dangerous and confirm=true.",
			InputSchema: object(map[string]any{
				"project": stringProp("Project _id (x-project-id)."),
				"id":      stringProp("Server id."),
				"action":  stringProp("Power action, for example start, shutdown, reboot or force-stop."),
				"confirm": boolProp("Must be true. Confirms this mutating call."),
			}, "id", "action", "confirm"),
			ReadOnly:    false,
			Destructive: true,
			Dangerous:   true,
			Handler: func(ctx context.Context, args map[string]json.RawMessage) (json.RawMessage, error) {
				return s.power(ctx, args, "/v1/portal/compute/")
			},
		},
		{
			Name:        "scaleblade_server_action",
			Title:       "DANGEROUS: bare-metal server action",
			Description: "DANGEROUS: POST a bare-metal action such as reinstall or a power action under the compute service. Endpoint vocabulary is inferred from portal bundles; verify before use. Requires server flag --allow-dangerous and confirm=true.",
			InputSchema: object(map[string]any{
				"project": stringProp("Project _id (x-project-id)."),
				"id":      stringProp("Server id."),
				"action":  enumProp("Action path segment.", "reinstall", "power/start", "power/shutdown", "power/reboot", "power/force-stop"),
				"confirm": boolProp("Must be true. Confirms this mutating call."),
			}, "id", "action", "confirm"),
			ReadOnly:    false,
			Destructive: true,
			Dangerous:   true,
			Handler: func(ctx context.Context, args map[string]json.RawMessage) (json.RawMessage, error) {
				project, id, err := s.projectAndID(args)
				if err != nil {
					return nil, err
				}
				action, err := stringArg(args, "action")
				if err != nil {
					return nil, err
				}
				allowed := map[string]bool{
					"reinstall": true, "power/start": true, "power/shutdown": true,
					"power/reboot": true, "power/force-stop": true,
				}
				if !allowed[action] {
					return nil, fmt.Errorf("unsupported action %q", action)
				}
				segs, err := escapeSegments(action)
				if err != nil {
					return nil, err
				}
				path := "/v1/portal/compute/" + url.PathEscape(id) + "/" + segs
				return s.post(ctx, path, project)
			},
		},
	}
}

func (s *Server) power(ctx context.Context, args map[string]json.RawMessage, prefix string) (json.RawMessage, error) {
	project, id, err := s.projectAndID(args)
	if err != nil {
		return nil, err
	}
	action, err := stringArg(args, "action")
	if err != nil {
		return nil, err
	}
	if !actionPattern.MatchString(action) {
		return nil, fmt.Errorf("invalid action %q", action)
	}
	path := prefix + url.PathEscape(id) + "/power/" + url.PathEscape(action)
	return s.post(ctx, path, project)
}

func (s *Server) post(ctx context.Context, path, project string) (json.RawMessage, error) {
	key := newRequestID()
	resp, err := s.client.PostJSON(ctx, path, project, map[string]string{"idempotency_key": key})
	if err != nil {
		return nil, err
	}
	if len(resp.Data) > 0 && string(resp.Data) != "null" {
		return resp.Data, nil
	}
	return json.Marshal(map[string]any{"status": "accepted", "http_status": resp.Status})
}

func (s *Server) get(ctx context.Context, path, project string) (json.RawMessage, error) {
	data, err := s.client.GetRaw(ctx, path, project)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return json.RawMessage("null"), nil
	}
	return data, nil
}

func (s *Server) requireProject(args map[string]json.RawMessage) (string, error) {
	project := optionalString(args, "project")
	if project == "" {
		project = s.defaultProject
	}
	if project == "" {
		return "", errors.New("project is required: pass the project argument or start scaleblade-mcp with --project")
	}
	return project, nil
}

func (s *Server) projectAndID(args map[string]json.RawMessage) (project, id string, err error) {
	project, err = s.requireProject(args)
	if err != nil {
		return "", "", err
	}
	id, err = stringArg(args, "id")
	if err != nil {
		return "", "", err
	}
	return project, id, nil
}

func escapeSegments(action string) (string, error) {
	parts := splitSlash(action)
	if len(parts) == 0 {
		return "", errors.New("empty action")
	}
	out := ""
	for i, p := range parts {
		if p == "" {
			return "", fmt.Errorf("invalid action %q", action)
		}
		if i > 0 {
			out += "/"
		}
		out += url.PathEscape(p)
	}
	return out, nil
}

func splitSlash(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

func stringArg(args map[string]json.RawMessage, name string) (string, error) {
	raw, ok := args[name]
	if !ok {
		return "", fmt.Errorf("missing required argument %q", name)
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", fmt.Errorf("argument %q must be a string", name)
	}
	if v == "" {
		return "", fmt.Errorf("argument %q must not be empty", name)
	}
	return v, nil
}

func optionalString(args map[string]json.RawMessage, name string) string {
	raw, ok := args[name]
	if !ok {
		return ""
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	return v
}

func boolArg(args map[string]json.RawMessage, name string) (bool, error) {
	raw, ok := args[name]
	if !ok {
		return false, fmt.Errorf("missing required argument %q", name)
	}
	var v bool
	if err := json.Unmarshal(raw, &v); err != nil {
		return false, fmt.Errorf("argument %q must be a boolean", name)
	}
	return v, nil
}

func intArg(args map[string]json.RawMessage, name string) (int64, bool) {
	raw, ok := args[name]
	if !ok {
		return 0, false
	}
	var v int64
	if err := json.Unmarshal(raw, &v); err != nil || v < 0 {
		return 0, false
	}
	return v, true
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return strconv.FormatInt(time.Now().UnixNano(), 16)
}

func object(props map[string]any, required ...string) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
	schema := map[string]any{
		"type":                 "object",
		"properties":           props,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func stringProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func optionalProjectProp() map[string]any {
	return map[string]any{"type": "string", "description": "Project _id (x-project-id). Optional; defaults to the server's --project value."}
}

func boolProp(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

func intProp(desc string) map[string]any {
	return map[string]any{"type": "integer", "minimum": 0, "description": desc}
}

func enumProp(desc string, values ...string) map[string]any {
	return map[string]any{"type": "string", "description": desc, "enum": values}
}
