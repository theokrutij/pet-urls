package shortener

import (
	"strings"
	"testing"
)

// TODO: update test suite
func TestNormalizeHTTP_ValidInputs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want URL
	}{
		{"domain bare", "example.com", "http://example.com"},
		{"IPv4 bare", "127.0.0.1", "http://127.0.0.1"},
		{"IPv6 full, bare", "[2001:0db8:85a3:0000:0000:8a2e:0370:7334]", "http://[2001:0db8:85a3:0000:0000:8a2e:0370:7334]"},
		{"IPv6 short, bare", "[::1]", "http://[::1]"},
		{"IPv4 mapped IPv6, bare", "[::ffff:127.0.0.1]", "http://[::ffff:127.0.0.1]"},

		{"http scheme", "http://example.com", "http://example.com"},
		{"https scheme", "https://example.com", "https://example.com"},
		{"uppercase scheme", "HTTP://EXAMPLE.COM", "http://EXAMPLE.COM"},

		{"domain with scheme and port", "http://example.com:1234", "http://example.com:1234"},
		{"IPv4 with scheme and port", "http://127.0.0.1", "http://127.0.0.1"},
		{"IPv6 with port", "http://[::1]:1234", "http://[::1]:1234"},

		{"unicode path (Cyrillic)", "example.com/привет", "http://example.com/%D0%BF%D1%80%D0%B8%D0%B2%D0%B5%D1%82"},
		{"URL with encoded space in path", "example.com/space%20here", "http://example.com/space%20here"},
		{"URL with encoded Cyrillic in path", "example.com/%D0%BF%D1%80%D0%B8%D0%B2%D0%B5%D1%82", "http://example.com/%D0%BF%D1%80%D0%B8%D0%B2%D0%B5%D1%82"},

		{"URL with query string", "example.com/search?q=hello%20world", "http://example.com/search?q=hello%20world"},

		{"Valid domain with fragment", "example.com/path#section1", "http://example.com/path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeHTTP(tt.in)
			if err != nil {
				t.Fatalf("toHTTPURL(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("toHTTPURL(%q) = %q; want %q", tt.in, got, tt.want)
			}

		})
	}
}

func TestToHTTPURL_InvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"empty_input", ""},
		{"too long input", "http://" + strings.Repeat("a", 1994)},
		{"missing_host", "http://"},
		{"triple_slash", "http:///example.com"},

		{"user_info_in_host", "http://user@example.com"},

		{"mailto", "mailto:someone@example.com"},
		{"ftp", "ftp://example.com"},
		{"relative_url", "//example.com"},

		{"leading dot", ".a"},

		{"non IPv6 with square brackets", "http://[example.com]"},
		{"IPv6 without square brackets", "2001:0db8:85a3:0000:0000:8a2e:0370:7334"},

		{"unicode domain (Cyrillic)", "http://пример.рф"},
		{"leading hyphen", "-example.com"},
		{"trailing hyphen", "example.com-"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeHTTP(tt.in)
			if err == nil {
				t.Fatalf("toHTTPURL(%.100s) = %.100s; expected error", tt.in, got)
			}
		})
	}
}
