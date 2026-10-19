package api

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCookieFile(t *testing.T, dir, body string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, "cookies.json")
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseCookieJSONShapes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
		want string
	}{
		{"object", `{"domain":"example.test","cookies":{"b":"2","a":"1"}}`, "a=1; b=2"},
		{"array", `{"cookies":[{"name":"b","value":"2"},{"name":"a","value":"1"}]}`, "a=1; b=2"},
		{"flat", `{"domain":"example.test","b":"2","a":"1"}`, "a=1; b=2"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseCookieJSON([]byte(tc.body))
			if err != nil {
				t.Fatalf("parseCookieJSON: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseCookieJSONRejectsInvalid(t *testing.T) {
	t.Parallel()
	cases := []string{
		``,
		`not json`,
		`{"cookies":{}}`,
		`{"cookies":[{"name":"","value":"x"}]}`,
		`{"cookies":{"a":1}}`,
		`{"cookies":{"a":"line\nbreak"}}`,
		`{"cookies":{"a b":"x"}}`,
	}
	for _, body := range cases {
		if _, err := parseCookieJSON([]byte(body)); err == nil {
			t.Fatalf("expected error for %q", body)
		}
	}
}

func TestLoadCookieFileMode(t *testing.T) {
	dir := t.TempDir()
	path := writeCookieFile(t, dir, `{"cookies":{"session":"super-secret-value"}}`, 0o600)
	header, source, err := loadCookieFile(path)
	if err != nil {
		t.Fatalf("loadCookieFile: %v", err)
	}
	if header != "session=super-secret-value" {
		t.Fatalf("unexpected header %q", header)
	}
	if source != path {
		t.Fatalf("unexpected source %q", source)
	}

	badPath := writeCookieFile(t, dir, `{"cookies":{"session":"super-secret-value"}}`, 0o644)
	_, _, err = loadCookieFile(badPath)
	if err == nil {
		t.Fatal("expected permission error")
	}
	if !strings.Contains(err.Error(), "chmod 600") {
		t.Fatalf("error should explain chmod: %v", err)
	}
	if strings.Contains(err.Error(), "super-secret-value") {
		t.Fatal("error leaked the cookie value")
	}
}

func TestResolveCookiePrecedence(t *testing.T) {
	dir := t.TempDir()
	path := writeCookieFile(t, dir, `{"cookies":{"a":"1"}}`, 0o600)
	t.Setenv(EnvCookieFile, path)
	t.Setenv(EnvCookie, "ignored=yes")
	header, _, err := ResolveCookie("", "")
	if err != nil {
		t.Fatal(err)
	}
	if header != "a=1" {
		t.Fatalf("got %q", header)
	}
}

func TestResolveCookieLiteral(t *testing.T) {
	t.Setenv(EnvCookieFile, "")
	t.Setenv(EnvCookie, "one=1; two=2")
	header, source, err := ResolveCookie("", "")
	if err != nil {
		t.Fatal(err)
	}
	if header != "one=1; two=2" || source != EnvCookie {
		t.Fatalf("got header %q source %q", header, source)
	}
}

func TestResolveCookieMissing(t *testing.T) {
	t.Setenv(EnvCookieFile, "")
	t.Setenv(EnvCookie, "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, _, err := ResolveCookie("", "")
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("got %v, want ErrNoCredentials", err)
	}
}
