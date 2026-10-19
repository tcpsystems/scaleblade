package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testCookie = "session=test-value"

func newTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := New(Config{BaseURL: srv.URL, CookieHeader: testCookie, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return c, srv
}

func TestGetJSONEnvelopeAndHeaders(t *testing.T) {
	t.Parallel()
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Cookie"); got != testCookie {
			t.Errorf("Cookie header = %q", got)
		}
		if got := r.Header.Get("x-project-id"); got != "project-1" {
			t.Errorf("x-project-id = %q", got)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		_, _ = w.Write([]byte(`{"data":{"ok":true,"items":[1,2]}}`))
	}))
	var out struct {
		OK    bool  `json:"ok"`
		Items []int `json:"items"`
	}
	if err := c.GetJSON(context.Background(), "/v1/portal/projects", "project-1", &out); err != nil {
		t.Fatal(err)
	}
	if !out.OK || len(out.Items) != 2 {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestErrorEnvelope(t *testing.T) {
	t.Parallel()
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"not allowed","request_id":"req-1"}}`))
	}))
	_, err := c.Get(context.Background(), "/v1/admin/projects", "")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("got %T %v, want *APIError", err, err)
	}
	if apiErr.Status != 403 || apiErr.Code != "forbidden" || apiErr.Message != "not allowed" || apiErr.RequestID != "req-1" {
		t.Fatalf("unexpected APIError: %+v", apiErr)
	}
}

func TestNonEnvelopeError(t *testing.T) {
	t.Parallel()
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`upstream exploded session=test-value`))
	}))
	_, err := c.Get(context.Background(), "/v1/portal/billing", "")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("got %T %v", err, err)
	}
	if apiErr.Status != http.StatusBadGateway || apiErr.Code != "http_error" {
		t.Fatalf("unexpected error: %+v", apiErr)
	}
}

func TestNoContent(t *testing.T) {
	t.Parallel()
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	resp, err := c.Do(context.Background(), http.MethodPost, "/v1/portal/tasks/abc/cancel", "", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != http.StatusNoContent {
		t.Fatalf("status = %d", resp.Status)
	}
}

func TestPostJSON(t *testing.T) {
	t.Parallel()
	var gotBody map[string]string
	var gotMethod string
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"data":{"accepted":true}}`))
	}))
	resp, err := c.PostJSON(context.Background(), "/v1/portal/virtualization/virtual-machines/vm1/power/start", "p1", map[string]string{"idempotency_key": "abc"})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost || gotBody["idempotency_key"] != "abc" {
		t.Fatalf("method=%q body=%v", gotMethod, gotBody)
	}
	if string(resp.Data) != `{"accepted":true}` {
		t.Fatalf("data = %s", resp.Data)
	}
}

func TestTimeoutDoesNotLeakCookie(t *testing.T) {
	t.Parallel()
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	c.http.Timeout = 20 * time.Millisecond
	_, err := c.Get(context.Background(), "/v1/auth/me", "")
	if err == nil {
		t.Fatal("expected timeout")
	}
	if strings.Contains(err.Error(), "test-value") {
		t.Fatal("error leaked cookie")
	}
	if strings.Contains(c.String(), "test-value") {
		t.Fatal("String leaked cookie")
	}
}

func TestRedirectRefused(t *testing.T) {
	t.Parallel()
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("redirect target must not be called")
	}))
	defer other.Close()
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/v1/auth/me", http.StatusFound)
	}))
	if _, err := c.Get(context.Background(), "/v1/auth/me", ""); err == nil {
		t.Fatal("expected redirect rejection")
	}
}

func TestValidatePath(t *testing.T) {
	t.Parallel()
	valid := []string{
		"/v1/auth/me",
		"/v1/portal/network?limit=10&offset=0",
		"/v1/portal/compute/os-profiles?kind=operating_system",
	}
	for _, path := range valid {
		if err := ValidatePath(path); err != nil {
			t.Errorf("ValidatePath(%q) = %v", path, err)
		}
	}
	invalid := []string{
		"",
		"/",
		"/v2/auth/me",
		"/v1",
		"https://evil.test/v1/auth/me",
		"/v1/../secret",
		"/v1/auth/me#frag",
		"/v1/auth/me\r\nHost: evil.test",
	}
	for _, path := range invalid {
		if err := ValidatePath(path); err == nil {
			t.Errorf("ValidatePath(%q) unexpectedly succeeded", path)
		}
	}
}

func TestMissingCookieFile(t *testing.T) {
	t.Setenv(EnvCookieFile, "")
	t.Setenv(EnvCookie, "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := New(Config{}); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("got %v, want ErrNoCredentials", err)
	}
}
