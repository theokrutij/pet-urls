package shortener

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"unicode/utf8"
)

func normalizeHTTP(raw string) (URL, error) {
	if utf8.RuneCountInString(raw) > 2000 {
		return "", errors.New("too long url")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("invalid url")
	}
	if !urlIsValid(u) {
		return "", errors.New("invalid url")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Fragment = ""

	return URL(u.String()), nil
}

func urlIsValid(u *url.URL) bool {
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if u.Opaque != "" {
		return false
	}
	if u.User != nil {
		return false
	}

	// URL.Hostname() trims square brackets regardless of whether host is
	// IPv6 or not, which allows invalid urls like http://[example.com]
	hostname, _ := strings.CutSuffix(u.Host, ":"+u.Port())

	if !isValidFQDN(hostname) && !isIP(hostname) {
		return false
	}
	return true
}

func isValidFQDN(s string) bool {
	// Full domain name is limited to 255 bytes.
	if utf8.RuneCountInString(s) > 255 {
		return false
	}
	// Trailing dot is part of canonical FQDN. If missing, it is implied.
	s, _ = strings.CutSuffix(s, ".")

	labels := strings.Split(s, ".")
	for _, label := range labels {
		if !isValidFQDNLabel(label) {
			return false
		}
	}

	return true
}

func isValidFQDNLabel(label string) bool {
	// The length of each label must be between 1 and 63 bytes.
	if len(label) == 0 || len(label) > 63 {
		return false
	}
	for _, r := range label {
		// Label must only use english letters, digits and hyphens.
		if !isValidFQDNLabelChar(r) {
			return false
		}
	}
	// Label cannot start or end with a hyphen.
	if label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	return true
}

func isValidFQDNLabelChar(r rune) bool {
	// English letters, digits, and a hyphen are allowed.
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		(r == '-')
}

func isIP(s string) bool {
	// Trim square brackets to check for potential IPv6
	if strings.ContainsRune(s, ':') && s[0] == '[' && s[len(s)-1] == ']' {
		s = s[1 : len(s)-1]
	}
	ip := net.ParseIP(s)
	return ip != nil
}
