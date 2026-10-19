package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	// EnvCookie may contain either a literal Cookie header value or a path to
	// a cookie JSON file. A literal value is used as-is.
	EnvCookie = "SCALEBLADE_COOKIE"

	// EnvCookieFile points at a cookie JSON file. It takes precedence over
	// EnvCookie.
	EnvCookieFile = "SCALEBLADE_COOKIE_FILE"
)

// ErrNoCredentials is returned when no cookie source produces a usable
// session. It never contains cookie values.
var ErrNoCredentials = errors.New("no Scaleblade credentials found")

// DefaultCookiePath returns the documented default cookie location. It is
// $XDG_CONFIG_HOME/scaleblade/cookies.json when XDG_CONFIG_HOME is set, and
// $HOME/.config/scaleblade/cookies.json otherwise.
func DefaultCookiePath() string {
	if dir := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); dir != "" {
		return filepath.Join(dir, "scaleblade", "cookies.json")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".config", "scaleblade", "cookies.json")
	}
	return filepath.Join(home, ".config", "scaleblade", "cookies.json")
}

// ResolveCookie returns a Cookie header value and a human-readable source
// description. explicitHeader wins, then explicitFile, then environment, then
// the default file. It never returns or embeds cookie values in errors.
func ResolveCookie(explicitHeader, explicitFile string) (header, source string, err error) {
	if explicitHeader != "" {
		return normalizeCookieHeader(explicitHeader, "explicit cookie header")
	}
	if explicitFile != "" {
		return loadCookieFile(explicitFile)
	}
	if path := strings.TrimSpace(os.Getenv(EnvCookieFile)); path != "" {
		return loadCookieFile(path)
	}
	if v := strings.TrimSpace(os.Getenv(EnvCookie)); v != "" {
		// Accept a file path in EnvCookie for convenience; otherwise treat the
		// value as a literal Cookie header.
		if info, statErr := os.Stat(v); statErr == nil && info.Mode().IsRegular() {
			return loadCookieFile(v)
		}
		return normalizeCookieHeader(v, "SCALEBLADE_COOKIE")
	}
	path := DefaultCookiePath()
	if _, statErr := os.Stat(path); statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return "", "", fmt.Errorf("%w: %s does not exist; see README.md for cookie extraction", ErrNoCredentials, path)
		}
		return "", "", fmt.Errorf("%w: cannot stat %s: %v", ErrNoCredentials, path, statErr)
	}
	return loadCookieFile(path)
}

func normalizeCookieHeader(value, source string) (string, string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", fmt.Errorf("%w: empty cookie value from %s", ErrNoCredentials, source)
	}
	if strings.ContainsAny(value, "\r\n\x00") {
		return "", "", fmt.Errorf("%w: cookie value from %s contains a control character", ErrNoCredentials, source)
	}
	if !strings.Contains(value, "=") {
		return "", "", fmt.Errorf("%w: cookie value from %s is not a name=value pair", ErrNoCredentials, source)
	}
	return value, source, nil
}

func loadCookieFile(path string) (string, string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", "", fmt.Errorf("%w: cannot read cookie file %s: %v", ErrNoCredentials, path, err)
	}
	if !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("%w: cookie path %s is not a regular file", ErrNoCredentials, path)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			return "", "", fmt.Errorf("%w: cookie file %s has mode %04o; run chmod 600 %s", ErrNoCredentials, path, perm, path)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("%w: cannot read cookie file %s: %v", ErrNoCredentials, path, err)
	}
	header, err := parseCookieJSON(data)
	if err != nil {
		return "", "", fmt.Errorf("%w: %s: %v", ErrNoCredentials, path, err)
	}
	return header, path, nil
}

// parseCookieJSON accepts the shapes commonly exported from browser tooling:
//
//	{"domain": "...", "cookies": {"name": "value", ...}}
//	{"cookies": [{"name": "name", "value": "value"}, ...]}
//	{"name": "value", ...}
//
// Values must be strings. Returned names are sorted for deterministic headers.
func parseCookieJSON(data []byte) (string, error) {
	type pair struct{ name, value string }
	var pairs []pair

	var outer map[string]json.RawMessage
	if err := json.Unmarshal(data, &outer); err != nil {
		return "", errors.New("cookie file is not a valid JSON object")
	}

	rawCookies, hasCookies := outer["cookies"]
	if hasCookies {
		var object map[string]string
		if err := json.Unmarshal(rawCookies, &object); err == nil {
			for name, value := range object {
				pairs = append(pairs, pair{name, value})
			}
		} else {
			var list []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			}
			if err := json.Unmarshal(rawCookies, &list); err != nil {
				return "", errors.New("cookies must be an object of name/value strings or an array of {name,value}")
			}
			for _, c := range list {
				pairs = append(pairs, pair{c.Name, c.Value})
			}
		}
	} else {
		var flat map[string]string
		if err := json.Unmarshal(data, &flat); err != nil {
			return "", errors.New("cookie file must contain a cookies object")
		}
		for name, value := range flat {
			if name == "domain" {
				continue
			}
			pairs = append(pairs, pair{name, value})
		}
	}

	if len(pairs) == 0 {
		return "", errors.New("cookie file contains no cookies")
	}

	sort.Slice(pairs, func(i, j int) bool { return pairs[i].name < pairs[j].name })

	parts := make([]string, 0, len(pairs))
	seen := make(map[string]struct{}, len(pairs))
	for _, p := range pairs {
		if err := validateCookie(p.name, p.value); err != nil {
			return "", err
		}
		if _, dup := seen[p.name]; dup {
			return "", fmt.Errorf("duplicate cookie name %q", p.name)
		}
		seen[p.name] = struct{}{}
		parts = append(parts, p.name+"="+p.value)
	}
	return strings.Join(parts, "; "), nil
}

func validateCookie(name, value string) error {
	if name == "" {
		return errors.New("cookie with empty name")
	}
	for _, r := range name {
		if !isCookieTokenRune(r) {
			return fmt.Errorf("invalid cookie name %q", name)
		}
	}
	if strings.ContainsAny(value, "\r\n\x00;") {
		return fmt.Errorf("cookie %q contains a forbidden control or separator character", name)
	}
	return nil
}

func isCookieTokenRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case strings.ContainsRune("!#$%&'*+-.^_`|~", r):
		return true
	default:
		return false
	}
}
