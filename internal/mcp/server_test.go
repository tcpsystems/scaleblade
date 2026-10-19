package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tcpsystems/scaleblade/internal/api"
)

func newMCPServer(t *testing.T, handler http.Handler, allowDangerous bool, defaultProject string) *Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client, err := api.New(api.Config{
		BaseURL:      srv.URL,
		CookieHeader: "session=test-value",
		Timeout:      2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(Options{
		Client:         client,
		Version:        "test",
		DefaultProject: defaultProject,
		AllowDangerous: allowDangerous,
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type rpcOut struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

func call(t *testing.T, s *Server, method string, id int, params any) rpcOut {
	t.Helper()
	var paramsJSON []byte
	if params != nil {
		var err error
		paramsJSON, err = json.Marshal(params)
		if err != nil {
			t.Fatal(err)
		}
	}
	frame := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`, id, method, string(paramsJSON))
	if params == nil {
		frame = fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q}`, id, method)
	}
	raw := s.HandleMessage(context.Background(), []byte(frame))
	if raw == nil {
		t.Fatalf("%s returned no response", method)
	}
	var out rpcOut
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode response: %v (%s)", err, raw)
	}
	return out
}

func TestInitialize(t *testing.T) {
	s := newMCPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), false, "")
	out := call(t, s, "initialize", 1, map[string]any{"protocolVersion": "2025-03-26"})
	if out.Error != nil {
		t.Fatal(out.Error)
	}
	var result struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities"`
		ServerInfo      map[string]any `json:"serverInfo"`
		Instructions    string         `json:"instructions"`
	}
	if err := json.Unmarshal(out.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.ProtocolVersion != "2025-03-26" {
		t.Fatalf("protocolVersion = %q", result.ProtocolVersion)
	}
	if result.ServerInfo["name"] != "scaleblade" {
		t.Fatalf("serverInfo = %v", result.ServerInfo)
	}
	if !strings.Contains(result.Instructions, "not affiliated") {
		t.Fatalf("instructions missing disclaimer")
	}
}

func TestToolsList(t *testing.T) {
	s := newMCPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), false, "")
	out := call(t, s, "tools/list", 2, nil)
	if out.Error != nil {
		t.Fatal(out.Error)
	}
	var result struct {
		Tools []toolSpec `json:"tools"`
	}
	if err := json.Unmarshal(out.Result, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Tools) < 15 {
		t.Fatalf("only %d tools", len(result.Tools))
	}
	var foundDanger bool
	for _, tool := range result.Tools {
		if tool.Name == "scaleblade_vm_power" {
			foundDanger = true
			if !strings.Contains(tool.Description, "DISABLED") {
				t.Fatalf("dangerous tool not marked disabled: %q", tool.Description)
			}
		}
		if tool.Name == "scaleblade_auth_me" {
			if tool.Annotations["readOnlyHint"] != true {
				t.Fatalf("auth_me annotations = %v", tool.Annotations)
			}
		}
	}
	if !foundDanger {
		t.Fatal("vm power tool missing")
	}
}

func TestToolCallProjects(t *testing.T) {
	s := newMCPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/portal/projects" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"_id":"p1","label":"Synthetic"}]}`))
	}), false, "")
	out := call(t, s, "tools/call", 3, map[string]any{
		"name":      "scaleblade_projects_list",
		"arguments": map[string]any{},
	})
	if out.Error != nil {
		t.Fatal(out.Error)
	}
	var result callResult
	if err := json.Unmarshal(out.Result, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 || !strings.Contains(result.Content[0].Text, "p1") {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.IsError {
		t.Fatal("unexpected tool error")
	}
}

func TestToolCallRequiresProject(t *testing.T) {
	s := newMCPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request should not be sent")
	}), false, "")
	out := call(t, s, "tools/call", 4, map[string]any{
		"name":      "scaleblade_compute_list",
		"arguments": map[string]any{},
	})
	var result callResult
	if err := json.Unmarshal(out.Result, &result); err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content[0].Text, "project is required") {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestToolCallDefaultProject(t *testing.T) {
	s := newMCPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-project-id") != "default-project" {
			t.Errorf("project header = %q", r.Header.Get("x-project-id"))
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	}), false, "default-project")
	out := call(t, s, "tools/call", 5, map[string]any{
		"name":      "scaleblade_vms_list",
		"arguments": map[string]any{},
	})
	var result callResult
	if err := json.Unmarshal(out.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %+v", result)
	}
}

func TestToolCallRequestPathValidation(t *testing.T) {
	s := newMCPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request should not be sent")
	}), false, "")
	out := call(t, s, "tools/call", 6, map[string]any{
		"name":      "scaleblade_request",
		"arguments": map[string]any{"path": "https://evil.test/v1/auth/me"},
	})
	var result callResult
	if err := json.Unmarshal(out.Result, &result); err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected tool error")
	}
}

func TestDangerousDisabled(t *testing.T) {
	s := newMCPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request should not be sent")
	}), false, "")
	out := call(t, s, "tools/call", 7, map[string]any{
		"name": "scaleblade_vm_power",
		"arguments": map[string]any{
			"project": "p1", "id": "vm1", "action": "reboot", "confirm": true,
		},
	})
	var result callResult
	if err := json.Unmarshal(out.Result, &result); err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content[0].Text, "--allow-dangerous") {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestDangerousRequiresConfirm(t *testing.T) {
	s := newMCPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request should not be sent")
	}), true, "")
	out := call(t, s, "tools/call", 8, map[string]any{
		"name": "scaleblade_vm_power",
		"arguments": map[string]any{
			"project": "p1", "id": "vm1", "action": "reboot",
		},
	})
	var result callResult
	if err := json.Unmarshal(out.Result, &result); err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content[0].Text, "confirm=true") {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestDangerousAllowed(t *testing.T) {
	var sawPost bool
	s := newMCPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/v1/portal/virtualization/virtual-machines/vm1/power/reboot" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("x-project-id") != "p1" {
			t.Errorf("project header = %q", r.Header.Get("x-project-id"))
		}
		sawPost = true
		_, _ = w.Write([]byte(`{"data":{"task_id":"t1"}}`))
	}), true, "")
	out := call(t, s, "tools/call", 9, map[string]any{
		"name": "scaleblade_vm_power",
		"arguments": map[string]any{
			"project": "p1", "id": "vm1", "action": "reboot", "confirm": true,
		},
	})
	var result callResult
	if err := json.Unmarshal(out.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %+v", result)
	}
	if !sawPost {
		t.Fatal("POST was not sent")
	}
	if !strings.Contains(result.Content[0].Text, "t1") {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestUnknownMethodAndParseError(t *testing.T) {
	s := newMCPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), false, "")
	out := call(t, s, "does/not/exist", 10, nil)
	if out.Error == nil || out.Error.Code != -32601 {
		t.Fatalf("unexpected response: %+v", out)
	}
	raw := s.HandleMessage(context.Background(), []byte(`{"jsonrpc":`))
	var parseErr rpcOut
	if err := json.Unmarshal(raw, &parseErr); err != nil {
		t.Fatal(err)
	}
	if parseErr.Error == nil || parseErr.Error.Code != -32700 {
		t.Fatalf("unexpected parse error: %+v", parseErr)
	}
}

func TestNotificationNoResponse(t *testing.T) {
	s := newMCPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), false, "")
	raw := s.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	if raw != nil {
		t.Fatalf("notification produced a response: %s", raw)
	}
}

func TestContentLengthFraming(t *testing.T) {
	s := newMCPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), false, "")
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`
	input := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	var out bytes.Buffer
	if err := s.Run(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	var resp rpcOut
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp); err != nil {
		t.Fatalf("decode: %v (%s)", err, out.String())
	}
	if resp.Error != nil {
		t.Fatal(resp.Error)
	}
	if string(resp.ID) != "1" {
		t.Fatalf("id = %s", resp.ID)
	}
}
