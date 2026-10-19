// Package mcp implements a dependency-free Model Context Protocol server over
// stdio for the reverse-engineered Scaleblade portal API.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/tcpsystems/scaleblade/internal/api"
)

const (
	serverName     = "scaleblade"
	serverTitle    = "Scaleblade portal (unofficial)"
	latestProtocol = "2025-06-18"
)

var supportedProtocols = map[string]bool{
	"2025-06-18": true,
	"2025-03-26": true,
	"2024-11-05": true,
}

var instructions = `Unofficial, reverse-engineered access to the Scaleblade provider portal API.

This server is not affiliated with Scaleblade and may break without notice. It
uses the session cookies from your browser; it never returns or logs them.
Read-only tools can be called freely. Power and reinstall tools are disabled
unless the server was started with --allow-dangerous and the call sets
"confirm": true. These tools mutate real infrastructure.

Project-scoped tools accept an optional "project" argument. If omitted, the
server's --project value (or SCALEBLADE_PROJECT) is used. List projects with
scaleblade_projects_list. Prefer specific tools over scaleblade_request.`

// Server is one MCP stdio session.
type Server struct {
	client         *api.Client
	version        string
	defaultProject string
	allowDangerous bool
	tools          []Tool
	byName         map[string]Tool
}

// Options configures a Server.
type Options struct {
	Client         *api.Client
	Version        string
	DefaultProject string
	AllowDangerous bool
}

// New builds a Server and registers its tools.
func New(opts Options) (*Server, error) {
	if opts.Client == nil {
		return nil, errors.New("mcp: client is required")
	}
	s := &Server{
		client:         opts.Client,
		version:        opts.Version,
		defaultProject: opts.DefaultProject,
		allowDangerous: opts.AllowDangerous,
		byName:         make(map[string]Tool),
	}
	s.tools = s.defineTools()
	for _, t := range s.tools {
		if _, dup := s.byName[t.Name]; dup {
			return nil, fmt.Errorf("mcp: duplicate tool %q", t.Name)
		}
		s.byName[t.Name] = t
	}
	return s, nil
}

// Run reads JSON-RPC messages from in and writes responses to out until EOF.
func (s *Server) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	reader := bufio.NewReaderSize(in, 64*1024)
	writer := bufio.NewWriter(out)
	defer writer.Flush()
	for {
		frame, err := readFrame(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		response := s.HandleMessage(ctx, frame)
		if response == nil {
			continue
		}
		if _, err := writer.Write(response); err != nil {
			return err
		}
		if err := writer.WriteByte('\n'); err != nil {
			return err
		}
		if err := writer.Flush(); err != nil {
			return err
		}
	}
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// HandleMessage processes one frame and returns the encoded response, or nil
// for notifications. It is exported for tests and embedding.
func (s *Server) HandleMessage(ctx context.Context, frame []byte) []byte {
	var req rpcRequest
	if err := json.Unmarshal(frame, &req); err != nil {
		return marshalResponse(rpcResponse{
			JSONRPC: "2.0",
			ID:      json.RawMessage("null"),
			Error:   &rpcError{Code: -32700, Message: "parse error"},
		})
	}
	id := req.ID
	if len(id) == 0 {
		// Notifications never receive a response.
		return nil
	}
	if req.JSONRPC != "" && req.JSONRPC != "2.0" {
		return marshalResponse(rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: -32600, Message: "invalid JSON-RPC version"}})
	}

	switch req.Method {
	case "initialize":
		return marshalResponse(rpcResponse{JSONRPC: "2.0", ID: id, Result: s.initialize(req.Params)})
	case "ping":
		return marshalResponse(rpcResponse{JSONRPC: "2.0", ID: id, Result: struct{}{}})
	case "tools/list":
		return marshalResponse(rpcResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{"tools": s.toolList()}})
	case "tools/call":
		result, rpcErr := s.callTool(ctx, req.Params)
		if rpcErr != nil {
			return marshalResponse(rpcResponse{JSONRPC: "2.0", ID: id, Error: rpcErr})
		}
		return marshalResponse(rpcResponse{JSONRPC: "2.0", ID: id, Result: result})
	default:
		return marshalResponse(rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: -32601, Message: "method not found: " + req.Method}})
	}
}

func marshalResponse(resp rpcResponse) []byte {
	data, err := json.Marshal(resp)
	if err != nil {
		return []byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"internal error"}}`)
	}
	return data
}

func (s *Server) initialize(params json.RawMessage) map[string]any {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(params, &p)
	version := latestProtocol
	if supportedProtocols[p.ProtocolVersion] {
		version = p.ProtocolVersion
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities": map[string]any{
			"tools": map[string]any{"listChanged": false},
		},
		"serverInfo": map[string]any{
			"name":    serverName,
			"title":   serverTitle,
			"version": s.version,
		},
		"instructions": instructions,
	}
}

type toolSpec struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations,omitempty"`
}

func (s *Server) toolList() []toolSpec {
	out := make([]toolSpec, 0, len(s.tools))
	for _, t := range s.tools {
		list := t
		description := list.Description
		if list.Dangerous && !s.allowDangerous {
			description = "DISABLED (server started without --allow-dangerous). " + description
		}
		out = append(out, toolSpec{
			Name:        list.Name,
			Title:       list.Title,
			Description: description,
			InputSchema: list.InputSchema,
			Annotations: map[string]any{
				"readOnlyHint":    list.ReadOnly,
				"destructiveHint": list.Destructive,
				"idempotentHint":  list.Idempotent,
				"openWorldHint":   true,
			},
		})
	}
	return out
}

type callParams struct {
	Name      string                     `json:"name"`
	Arguments map[string]json.RawMessage `json:"arguments"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type callResult struct {
	Content []textContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

func (s *Server) callTool(ctx context.Context, params json.RawMessage) (*callResult, *rpcError) {
	var p callParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{Code: -32602, Message: "invalid tools/call params"}
	}
	tool, ok := s.byName[p.Name]
	if !ok {
		return nil, &rpcError{Code: -32602, Message: "unknown tool: " + p.Name}
	}
	if tool.Dangerous && !s.allowDangerous {
		return toolError("dangerous tool %q is disabled; start scaleblade-mcp with --allow-dangerous", tool.Name), nil
	}
	if tool.Dangerous {
		confirm, _ := boolArg(p.Arguments, "confirm")
		if !confirm {
			return toolError("refusing dangerous tool %q without confirm=true", tool.Name), nil
		}
	}
	if p.Arguments == nil {
		p.Arguments = map[string]json.RawMessage{}
	}
	data, err := tool.Handler(ctx, p.Arguments)
	if err != nil {
		return toolError("%v", err), nil
	}
	pretty, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return toolError("encode result: %v", err), nil
	}
	return &callResult{Content: []textContent{{Type: "text", Text: string(pretty)}}}, nil
}

func toolError(format string, args ...any) *callResult {
	return &callResult{
		Content: []textContent{{Type: "text", Text: fmt.Sprintf(format, args...)}},
		IsError: true,
	}
}

// readFrame supports newline-delimited JSON (current MCP stdio) and
// Content-Length framed JSON (older clients).
func readFrame(r *bufio.Reader) ([]byte, error) {
	for {
		line, err := r.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if strings.TrimSpace(trimmed) == "" {
			if errors.Is(err, io.EOF) {
				return nil, io.EOF
			}
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "content-length:") {
			n, parseErr := strconv.Atoi(strings.TrimSpace(trimmed[len("content-length:"):]))
			if parseErr != nil || n < 0 {
				return nil, errors.New("invalid Content-Length frame")
			}
			// Consume the remaining headers up to the blank line.
			for {
				h, herr := r.ReadString('\n')
				if herr != nil && !errors.Is(herr, io.EOF) {
					return nil, herr
				}
				if strings.TrimRight(h, "\r\n") == "" {
					break
				}
				if errors.Is(herr, io.EOF) {
					return nil, io.ErrUnexpectedEOF
				}
			}
			buf := make([]byte, n)
			if _, rerr := io.ReadFull(r, buf); rerr != nil {
				return nil, rerr
			}
			return buf, nil
		}
		if errors.Is(err, io.EOF) {
			return []byte(trimmed), nil
		}
		return []byte(trimmed), nil
	}
}
