package shortener

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"unicode/utf8"
)

// TODO:
//   - escape all NON-ASCII
//   - add HTTP schema
//   - remove fragment
//   - toLower
//   - cover with tests
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
	if !isFQDN(u.Host) && !isIP(u.Host) {
		return false
	}
	return true
}

// naive: only allow ascii letters in labels
func isFQDN(s string) bool {
	s, _ = strings.CutSuffix(s, ".")
	labels := strings.Split(s, ".")
	for _, label := range labels {
		if len(label) < 1 || len(label) > 63 {
			return false
		}
		for _, r := range label {
			if r < 'A' || (r > 'Z' && r < 'a') || r > 'z' {
				return false
			}
		}
	}

	return true
}

func isIP(s string) bool {
	s = strings.Trim(s, "[]")
	ip := net.ParseIP(s)
	return ip != nil && ip.To16() != nil
}
