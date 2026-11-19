package cli

import "testing"

// TestParseHostPortIPv6 pins bracketed IPv6 parsing, which the previous
// strings.Split("host:port") implementation rejected as too many segments.
func TestParseHostPortIPv6(t *testing.T) {
	host, port, err := parseHostPort("[::1]:8080")
	if err != nil {
		t.Fatalf("parseHostPort IPv6: %v", err)
	}
	if host != "::1" {
		t.Errorf("host = %q, want ::1", host)
	}
	if port != 8080 {
		t.Errorf("port = %d, want 8080", port)
	}
}

// TestParseHostPortIPv4AndEmpty preserves the existing accepted forms.
func TestParseHostPortIPv4AndEmpty(t *testing.T) {
	host, port, err := parseHostPort("127.0.0.1:9300")
	if err != nil || host != "127.0.0.1" || port != 9300 {
		t.Fatalf("IPv4 parse = (%q, %d, %v)", host, port, err)
	}
	host, port, err = parseHostPort("")
	if err != nil || host != "" || port != -1 {
		t.Fatalf("empty parse = (%q, %d, %v)", host, port, err)
	}
}
