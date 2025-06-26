package shortener

import (
	"fmt"
	"net/url"
	"strings"
)

func normalizeHTTP(raw string) (URL, error) {
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid url")
	}
	if u.Scheme == "" {
		raw = "http://" + raw
		u, err = url.Parse(raw)
		if err != nil {
			return "", fmt.Errorf("invalid url")
		}
	}
	if !urlIsValid(u) {
		return "", fmt.Errorf("invalid url")
	}
	u.Scheme = strings.ToLower(u.Scheme)

	return URL(u.String()), nil
}

// check that Host is either ipv4 or ipv6 or FQDN
// limit length?
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
	if u.Host == "" {
		return false
	}
	return true
}
