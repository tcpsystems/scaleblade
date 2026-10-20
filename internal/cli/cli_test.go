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
	"time"
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

func TestVersionCommand(t *testing.T) {
	code, out, errOut := runCLI(t, "version")
	if code != 0 {
		t.Fatalf("exit %d, stderr %s", code, errOut)
	}
	if !strings.Contains(out, "scaleblade test") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestAuthCheckOK(t *testing.T) {
	testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/me" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":{"role":"portal","expires_at":"` + futureStamp() + `"}}`))
	}))
	code, out, errOut := runCLI(t, "auth", "check")
	if code != 0 {
		t.Fatalf("exit %d, stderr %s", code, errOut)
	}
	if !strings.Contains(out, "ok") || !strings.Contains(out, "portal") {
		t.Fatalf("unexpected output: %s", out)
	}
	if strings.Contains(out+errOut, "test-value") {
		t.Fatal("cookie leaked into output")
	}
}

func TestAuthCheckJSON(t *testing.T) {
	testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"role":"portal","expires_at":"` + futureStamp() + `"}}`))
	}))
	code, out, errOut := runCLI(t, "auth", "check", "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr %s", code, errOut)
	}
	var st map[string]any
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("output is not JSON: %v: %s", err, out)
	}
	if st["authenticated"] != true {
		t.Fatalf("authenticated = %v", st["authenticated"])
	}
}

func TestAuthCheckExpiredIsExit3(t *testing.T) {
	testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"Sign in is required."}}`))
	}))
	code, _, errOut := runCLI(t, "auth", "check")
	if code != 3 {
		t.Fatalf("exit = %d, want 3; stderr %s", code, errOut)
	}
	if !strings.Contains(errOut, "refresh") || !strings.Contains(errOut, "cookie") {
		t.Fatalf("stderr should tell the operator how to refresh: %s", errOut)
	}
}

func TestAuthCheckServerErrorIsGeneric(t *testing.T) {
	testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"code":"upstream"}}`))
	}))
	code, _, _ := runCLI(t, "auth", "check")
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (not a session problem)", code)
	}
}

func TestAuthCheckMissingCookieIsExit3(t *testing.T) {
	t.Setenv("SCALEBLADE_COOKIE", "")
	t.Setenv("SCALEBLADE_COOKIE_FILE", "/nonexistent/cookies.json")
	code, _, errOut := runCLI(t, "auth", "check")
	if code != 3 {
		t.Fatalf("exit = %d, want 3; stderr %s", code, errOut)
	}
}

func TestTasksStuckDetectsStalled(t *testing.T) {
	testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
			{"_id":"t1","display_id":"TASK-1","type":"reinstall_vm","state":"running","progress":50,"updated_at":"` + pastStamp(3*time.Hour) + `"},
			{"_id":"t2","display_id":"TASK-2","type":"start_virtual_machine","state":"succeeded","progress":100,"updated_at":"` + pastStamp(5*time.Hour) + `"}
		]}`))
	}))
	code, out, errOut := runCLI(t, "tasks", "--stuck", "20m")
	if code != 4 {
		t.Fatalf("exit = %d, want 4; stderr %s", code, errOut)
	}
	if !strings.Contains(out, "TASK-1") {
		t.Fatalf("stalled task missing from output: %s", out)
	}
	if strings.Contains(out, "TASK-2") && strings.Contains(errOut, "TASK-2") {
		t.Fatalf("terminal task counted as stalled: %s %s", out, errOut)
	}
	if !strings.Contains(errOut, "no progress") {
		t.Fatalf("stderr should summarise the stall: %s", errOut)
	}
}

func TestTasksStuckAllHealthy(t *testing.T) {
	testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
			{"_id":"t1","display_id":"TASK-1","type":"reinstall_vm","state":"running","progress":50,"updated_at":"` + pastStamp(1*time.Minute) + `"}
		]}`))
	}))
	code, out, errOut := runCLI(t, "tasks", "--stuck", "20m")
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr %s", code, errOut)
	}
	if !strings.Contains(out, "no task has been in flight") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestTasksStuckJSONStillExits4(t *testing.T) {
	testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"_id":"t1","state":"running","updated_at":"` + pastStamp(3*time.Hour) + `"}]}`))
	}))
	code, out, _ := runCLI(t, "tasks", "--json", "--stuck", "20m")
	if code != 4 {
		t.Fatalf("exit = %d, want 4", code)
	}
	if !strings.Contains(out, `"_id"`) && !strings.Contains(out, `"t1"`) {
		t.Fatalf("JSON payload should still be printed: %s", out)
	}
}

func TestTasksStuckRejectsBadDuration(t *testing.T) {
	var calls int32
	testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
	}))
	code, _, errOut := runCLI(t, "tasks", "--stuck", "soon")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatal("request should not be sent with a bad duration")
	}
	if !strings.Contains(errOut, "--stuck") {
		t.Fatalf("stderr should mention --stuck: %s", errOut)
	}
}

func futureStamp() string { return time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339) }

func pastStamp(d time.Duration) string {
	return time.Now().Add(-d).UTC().Format("2006-01-02T15:04:05.000Z07:00")
}
