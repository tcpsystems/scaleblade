package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func testServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	t.Setenv("SCALEBLADE_BASE_URL", srv.URL)
	t.Setenv("SCALEBLADE_COOKIE", "session=test-value")
	t.Setenv("SCALEBLADE_COOKIE_FILE", "")
	t.Setenv("SCALEBLADE_PROJECT", "")
	return srv
}

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(context.Background(), args, &out, &errOut, "test")
	return code, out.String(), errOut.String()
}

func TestAuthJSON(t *testing.T) {
	testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/me" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":{"session_id":"s1","actor_id":"a1","effective_actor_id":"a1","effective_actor_email":"user@example.test","role":"portal","impersonating":false,"expires_at":"2030-01-01T00:00:00Z"}}`))
	}))
	code, out, errOut := runCLI(t, "auth", "me", "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr %s", code, errOut)
	}
	if !strings.Contains(out, `"role"`) || !strings.Contains(out, `"portal"`) {
		t.Fatalf("unexpected output: %s", out)
	}
	if strings.Contains(out+errOut, "test-value") {
		t.Fatal("cookie leaked into output")
	}
}

func TestProjectsTable(t *testing.T) {
	testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"_id":"p1","label":"Synthetic","archived_at":null,"updated_at":"2030-01-01T00:00:00Z"}]}`))
	}))
	code, out, errOut := runCLI(t, "projects", "list")
	if code != 0 {
		t.Fatalf("exit %d, stderr %s", code, errOut)
	}
	if !strings.Contains(out, "p1") || !strings.Contains(out, "Synthetic") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestComputeRequiresProject(t *testing.T) {
	var calls int32
	testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	code, _, errOut := runCLI(t, "compute", "list")
	if code != 2 {
		t.Fatalf("exit = %d, want 2; stderr %s", code, errOut)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatal("request should not be sent without a project")
	}
	if !strings.Contains(errOut, "--project") {
		t.Fatalf("stderr should mention --project: %s", errOut)
	}
}

func TestVMPowerRequiresYes(t *testing.T) {
	var calls int32
	testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
	}))
	code, _, errOut := runCLI(t, "vm", "power", "vm1", "reboot", "--project", "p1")
	if code != 2 {
		t.Fatalf("exit = %d, want 2; stderr %s", code, errOut)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatal("request should not be sent without --yes")
	}
	if !strings.Contains(errOut, "--yes") {
		t.Fatalf("stderr should mention --yes: %s", errOut)
	}
}

func TestVMPowerPosts(t *testing.T) {
	var body map[string]string
	testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/v1/portal/virtualization/virtual-machines/vm1/power/reboot" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("x-project-id") != "p1" {
			t.Errorf("project header = %q", r.Header.Get("x-project-id"))
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"data":{"task_id":"task-1"}}`))
	}))
	code, out, errOut := runCLI(t, "vm", "power", "vm1", "reboot", "--project", "p1", "--yes", "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr %s", code, errOut)
	}
	if body["idempotency_key"] == "" {
		t.Fatal("missing idempotency_key")
	}
	if !strings.Contains(out, "task-1") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRequestRejectsNonV1(t *testing.T) {
	var calls int32
	testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
	}))
	code, _, _ := runCLI(t, "request", "/admin/projects")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatal("request should not be sent")
	}
}

func TestRequestGET(t *testing.T) {
	testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/v1/portal/tasks" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"_id":"t1"}]}`))
	}))
	code, out, errOut := runCLI(t, "request", "/v1/portal/tasks", "--project", "p1")
	if code != 0 {
		t.Fatalf("exit %d, stderr %s", code, errOut)
	}
	if !strings.Contains(out, "t1") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestBillingHuman(t *testing.T) {
	testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"invoices":[],"subscriptions":[],"credit_balance_pence":1234}}`))
	}))
	code, out, errOut := runCLI(t, "billing")
	if code != 0 {
		t.Fatalf("exit %d, stderr %s", code, errOut)
	}
	if !strings.Contains(out, "credit balance: 12.34") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestFlagsAfterSubcommand(t *testing.T) {
	testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-project-id") != "p1" {
			t.Errorf("project header = %q", r.Header.Get("x-project-id"))
		}
		_, _ = w.Write([]byte(`{"data":[{"_id":"vm1","label":"synthetic"}]}`))
	}))
	code, out, errOut := runCLI(t, "vms", "list", "--project", "p1", "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr %s", code, errOut)
	}
	if !strings.Contains(out, "vm1") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestUnknownCommand(t *testing.T) {
	code, _, errOut := runCLI(t, "frobnicate")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errOut, "unknown command") {
		t.Fatalf("unexpected stderr: %s", errOut)
	}
}

func TestHelp(t *testing.T) {
	code, out, _ := runCLI(t, "help")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, "vm power") || !strings.Contains(out, "unofficial") {
		t.Fatalf("unexpected help output")
	}
}
