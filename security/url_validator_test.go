package security

import (
	"net"
	"testing"
)

// ---------------------------------------------------------------------------
// isPrivateIP
// ---------------------------------------------------------------------------

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		private bool
	}{
		{"nil ip", "", true},
		{"loopback v4", "127.0.0.1", true},
		{"loopback v6", "::1", true},
		{"private 10.x", "10.0.0.1", true},
		{"private 172.16.x", "172.16.0.1", true},
		{"private 192.168.x", "192.168.1.1", true},
		{"link-local", "169.254.1.1", true},
		{"public google dns", "8.8.8.8", false},
		{"public cloudflare", "1.1.1.1", false},
		{"zero address", "0.0.0.1", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var ip net.IP
			if tc.ip != "" {
				ip = net.ParseIP(tc.ip)
			}
			got := isPrivateIP(ip)
			if got != tc.private {
				t.Fatalf("isPrivateIP(%q) = %v, want %v", tc.ip, got, tc.private)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ValidateURL
// ---------------------------------------------------------------------------

func TestValidateURL_EmptyURL(t *testing.T) {
	err := ValidateURL("")
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
	ssrf, ok := err.(*SSRFError)
	if !ok {
		t.Fatalf("expected SSRFError, got %T", err)
	}
	if ssrf.Reason != "empty URL" {
		t.Fatalf("unexpected reason: %s", ssrf.Reason)
	}
}

func TestValidateURL_UnsupportedScheme(t *testing.T) {
	tests := []string{
		"ftp://example.com",
		"file:///etc/passwd",
		"gopher://evil.com",
	}
	for _, u := range tests {
		err := ValidateURL(u)
		if err == nil {
			t.Fatalf("expected error for %q", u)
		}
	}
}

func TestValidateURL_BlockedHosts(t *testing.T) {
	blocked := []string{
		"http://localhost/secret",
		"http://127.0.0.1/admin",
		"http://0.0.0.0/",
		"http://metadata.google.internal/computeMetadata",
	}
	for _, u := range blocked {
		err := ValidateURL(u)
		if err == nil {
			t.Fatalf("expected blocked for %q", u)
		}
	}
}

func TestValidateURL_PrivateIP(t *testing.T) {
	privateURLs := []string{
		"http://10.0.0.1/api",
		"http://192.168.1.1/",
		"http://172.16.0.1/",
	}
	for _, u := range privateURLs {
		err := ValidateURL(u)
		if err == nil {
			t.Fatalf("expected blocked for private IP URL %q", u)
		}
	}
}

func TestValidateURL_ValidPublicHTTPS(t *testing.T) {
	err := ValidateURL("https://api.binance.com/api/v3/ticker/price")
	if err != nil {
		t.Fatalf("expected no error for public HTTPS URL, got: %v", err)
	}
}

func TestValidateURL_EmptyHostname(t *testing.T) {
	err := ValidateURL("http:///path")
	if err == nil {
		t.Fatal("expected error for empty hostname")
	}
}

// ---------------------------------------------------------------------------
// SSRFError
// ---------------------------------------------------------------------------

func TestSSRFError_ErrorMessage(t *testing.T) {
	e := &SSRFError{URL: "http://evil.com", Reason: "blocked"}
	msg := e.Error()
	if msg != "SSRF blocked: http://evil.com - blocked" {
		t.Fatalf("unexpected message: %s", msg)
	}
}
