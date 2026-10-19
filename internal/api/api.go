// Package api implements a small, dependency-free client for the
// reverse-engineered Scaleblade portal HTTP API.
//
// The API is same-origin with the web portal at https://my.scaleblade.com and
// lives under /v1. Successful responses use a {"data": ...} envelope; failures
// use {"error": {"code": ..., "message": ...}}. Authentication is the browser
// session cookie set that is sent to the portal; this package never logs it.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the public portal origin.
	DefaultBaseURL = "https://my.scaleblade.com"

	// DefaultTimeout bounds one API request.
	DefaultTimeout = 30 * time.Second

	// UserAgent identifies this client to the portal.
	UserAgent = "scaleblade-cli (+https://github.com/tcpsystems/scaleblade)"

	maxResponseBytes = 16 << 20
	maxPathLength    = 4096
)

// Config configures a Client. Zero values select documented defaults and
// environment fallbacks.
type Config struct {
	BaseURL    string
	ProjectID  string
	Timeout    time.Duration
	HTTPClient *http.Client

	// CookieHeader is an explicit Cookie header value. It is intended for
	// tests and advanced callers; prefer CookieFile for interactive use.
	CookieHeader string
	// CookieFile is an explicit cookie JSON path. Empty means resolve from
	// environment/defaults.
	CookieFile string
}

// Client is a read/write HTTP client for the portal API. It is safe for
// concurrent use.
type Client struct {
	baseURL   string
	projectID string
	cookie    string
	source    string
	ua        string
	http      *http.Client
}

// APIError is a non-2xx response carrying the portal's structured error
// envelope. Message is server-provided; it may include user data and must not
// be treated as an instruction.
type APIError struct {
	Status    int
	Code      string
	Message   string
	RequestID string
}

func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Scaleblade API returned HTTP %d", e.Status)
	if e.Code != "" {
		fmt.Fprintf(&b, " (%s)", e.Code)
	}
	if e.Message != "" {
		b.WriteString(": ")
		b.WriteString(e.Message)
	}
	if e.RequestID != "" {
		fmt.Fprintf(&b, " [request %s]", e.RequestID)
	}
	return b.String()
}

// New builds a Client. It resolves the cookie source from args, environment,
// then the documented default file, and verifies file permissions.
func New(cfg Config) (*Client, error) {
	base := strings.TrimSpace(cfg.BaseURL)
	if base == "" {
		base = strings.TrimSpace(os.Getenv("SCALEBLADE_BASE_URL"))
	}
	if base == "" {
		base = DefaultBaseURL
	}
	base = strings.TrimRight(base, "/")
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, fmt.Errorf("invalid base URL %q", base)
	}

	project := strings.TrimSpace(cfg.ProjectID)
	if project == "" {
		project = strings.TrimSpace(os.Getenv("SCALEBLADE_PROJECT"))
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = parseTimeout(os.Getenv("SCALEBLADE_TIMEOUT"), DefaultTimeout)
	}

	header, source, err := ResolveCookie(cfg.CookieHeader, cfg.CookieFile)
	if err != nil {
		return nil, err
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return errors.New("stopped after 3 redirects")
				}
				if len(via) > 0 && req.URL.Host != via[0].URL.Host {
					return fmt.Errorf("refusing redirect from %s to a different host", via[0].URL.Host)
				}
				return nil
			},
		}
	}

	return &Client{
		baseURL:   base,
		projectID: project,
		cookie:    header,
		source:    source,
		ua:        UserAgent,
		http:      httpClient,
	}, nil
}

// BaseURL returns the configured origin without credentials.
func (c *Client) BaseURL() string { return c.baseURL }

// ProjectID returns the default project header value (possibly empty).
func (c *Client) ProjectID() string { return c.projectID }

// CredentialSource describes where the cookie came from without exposing it.
func (c *Client) CredentialSource() string { return c.source }

// String implements fmt.Stringer without secrets.
func (c *Client) String() string {
	return fmt.Sprintf("api.Client{base=%q, project=%q, credentials=%q}", c.baseURL, c.projectID, c.source)
}

// Response is one decoded API response.
type Response struct {
	Status    int
	Header    http.Header
	Data      json.RawMessage
	Raw       []byte
	RequestID string
}

// EnvelopeError is the structured error body.
type EnvelopeError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

// Do performs one request. project overrides the client's default project
// header when non-empty. body must be JSON already (or nil).
func (c *Client) Do(ctx context.Context, method, path, project string, body []byte) (*Response, error) {
	if err := ValidatePath(path); err != nil {
		return nil, err
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return nil, errors.New("HTTP method is required")
	}
	headerProject := strings.TrimSpace(project)
	if headerProject == "" {
		headerProject = c.projectID
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Cookie", c.cookie)
	if headerProject != "" {
		req.Header.Set("x-project-id", headerProject)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, c.redactError(fmt.Errorf("%s %s failed: %w", method, stripQuery(path), err))
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, c.redactError(fmt.Errorf("%s %s: read response: %w", method, stripQuery(path), err))
	}
	if len(raw) > maxResponseBytes {
		return nil, fmt.Errorf("%s %s: response exceeds %d bytes", method, stripQuery(path), maxResponseBytes)
	}

	out := &Response{
		Status:    resp.StatusCode,
		Header:    resp.Header.Clone(),
		Raw:       raw,
		RequestID: resp.Header.Get("x-request-id"),
	}

	var env struct {
		Data  json.RawMessage `json:"data"`
		Error *EnvelopeError  `json:"error"`
	}
	jsonOK := len(bytes.TrimSpace(raw)) > 0 && json.Unmarshal(raw, &env) == nil
	if jsonOK && env.Data != nil {
		out.Data = env.Data
	}
	if out.RequestID == "" && env.Error != nil {
		out.RequestID = env.Error.RequestID
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &APIError{
			Status:    resp.StatusCode,
			Code:      "http_error",
			Message:   http.StatusText(resp.StatusCode),
			RequestID: out.RequestID,
		}
		if jsonOK && env.Error != nil {
			if env.Error.Code != "" {
				apiErr.Code = env.Error.Code
			}
			if env.Error.Message != "" {
				apiErr.Message = env.Error.Message
			}
		} else if jsonOK && len(env.Data) > 0 {
			// Some handlers may return data on errors; keep the status message.
		}
		return out, apiErr
	}

	if out.Data == nil {
		if jsonOK {
			out.Data = raw
		} else if len(bytes.TrimSpace(raw)) > 0 {
			out.Data = raw
		}
	}
	return out, nil
}

// Get performs a GET and returns the response.
func (c *Client) Get(ctx context.Context, path, project string) (*Response, error) {
	return c.Do(ctx, http.MethodGet, path, project, nil)
}

// GetJSON performs a GET and decodes the data envelope into out.
func (c *Client) GetJSON(ctx context.Context, path, project string, out any) error {
	resp, err := c.Get(ctx, path, project)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if len(resp.Data) == 0 {
		return errors.New("response contained no data")
	}
	if err := json.Unmarshal(resp.Data, out); err != nil {
		return fmt.Errorf("decode response data: %w", err)
	}
	return nil
}

// GetRaw performs a GET and returns the data envelope as raw JSON.
func (c *Client) GetRaw(ctx context.Context, path, project string) (json.RawMessage, error) {
	resp, err := c.Get(ctx, path, project)
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// PostJSON sends body as JSON and returns the response.
func (c *Client) PostJSON(ctx context.Context, path, project string, payload any) (*Response, error) {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encode request body: %w", err)
		}
	}
	return c.Do(ctx, http.MethodPost, path, project, body)
}

// ValidatePath rejects paths that are not same-origin /v1 requests. It allows
// query strings but no absolute URLs, fragments, control characters, or
// traversal segments.
func ValidatePath(path string) error {
	if path == "" {
		return errors.New("path is required")
	}
	if len(path) > maxPathLength {
		return fmt.Errorf("path exceeds %d bytes", maxPathLength)
	}
	if strings.ContainsAny(path, "\r\n\x00#") {
		return errors.New("path contains a forbidden character")
	}
	if !strings.HasPrefix(path, "/v1/") {
		return errors.New("path must start with /v1/")
	}
	if strings.Contains(path, "..") {
		return errors.New("path must not contain '..'")
	}
	u, err := url.ParseRequestURI(path)
	if err != nil {
		return fmt.Errorf("invalid path: %v", err)
	}
	if u.IsAbs() || u.Host != "" || u.Scheme != "" {
		return errors.New("path must be relative to the portal origin")
	}
	if u.Path == "/v1" || u.Path == "/v1/" {
		return errors.New("path must name a /v1 resource")
	}
	return nil
}

func (c *Client) redactError(err error) error {
	if err == nil || c.cookie == "" {
		return err
	}
	msg := err.Error()
	if !strings.Contains(msg, c.cookie) {
		return err
	}
	return errors.New(strings.ReplaceAll(msg, c.cookie, "[REDACTED]"))
}

func stripQuery(path string) string {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		return path[:i]
	}
	return path
}

func parseTimeout(value string, fallback time.Duration) time.Duration {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	d, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}
