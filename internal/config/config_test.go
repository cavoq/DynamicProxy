package config

import (
	"os"
	"reflect"
	"regexp"
	"testing"
	"time"
)

func TestParseList(t *testing.T) {
	input := "http://localhost:8080, *.example.com, api.svc.com, https://www.test.org/"
	expected := []string{"localhost:8080", "*.example.com", "api.svc.com", "www.test.org"}

	result := GetExceptions(input)

	if len(result) != len(expected) {
		t.Fatalf("expected %d items, got %d", len(expected), len(result))
	}

	for i, v := range result {
		if v != expected[i] {
			t.Errorf("expected %q at index %d, got %q", expected[i], i, v)
		}
	}
}

func TestIsException(t *testing.T) {
	exceptions := []string{"localhost", "*.example.com", "api.svc.com", "internal.local:8443", "*.corp.local:9443"}

	tests := []struct {
		host     string
		expected bool
	}{
		{"localhost", true},
		{"localhost:8080", true},
		{"www.example.com", true},
		{"example.com", false},
		{"api.example.com", true},
		{"api.example.com:443", true},
		{"api.svc.com", true},
		{"api.svc.com:8081", true},
		{"internal.local:8443", true},
		{"internal.local:443", false},
		{"api.corp.local:9443", true},
		{"api.corp.local:443", false},
		{"other.com", false},
		{"svc.api.com", false},
	}

	for _, tt := range tests {
		if IsException(tt.host, exceptions) != tt.expected {
			t.Errorf("IsException(%q) = %v; expected %v", tt.host, !tt.expected, tt.expected)
		}
	}
}

func TestBuildHostCandidates(t *testing.T) {
	tests := []struct {
		host     string
		expected []string
	}{
		{"api.example.com:443", []string{"api.example.com:443", "api.example.com"}},
		{"api.example.com", []string{"api.example.com"}},
		{"[::1]:8443", []string{"[::1]:8443", "::1"}},
		{"  ", nil},
	}

	for _, tt := range tests {
		got := buildHostCandidates(tt.host)
		if !reflect.DeepEqual(got, tt.expected) {
			t.Errorf("buildHostCandidates(%q) = %#v; expected %#v", tt.host, got, tt.expected)
		}
	}
}

func TestSplitOutPort(t *testing.T) {
	tests := []struct {
		host         string
		expectedHost string
		expectedOK   bool
	}{
		{"api.example.com:443", "api.example.com", true},
		{"api.example.com", "api.example.com", false},
		{"[::1]:8443", "::1", true},
		{"[::1]", "[::1]", false},
	}

	for _, tt := range tests {
		gotHost, gotOK := splitOutPort(tt.host)
		if gotHost != tt.expectedHost || gotOK != tt.expectedOK {
			t.Errorf("splitOutPort(%q) = (%q, %v); expected (%q, %v)", tt.host, gotHost, gotOK, tt.expectedHost, tt.expectedOK)
		}
	}
}

func TestNormalizeHostToken(t *testing.T) {
	tests := []struct {
		token    string
		expected string
	}{
		{" [::1] ", "::1"},
		{" Example.com ", "Example.com"},
		{"[]", "[]"},
	}

	for _, tt := range tests {
		got := normalizeHostToken(tt.token)
		if got != tt.expected {
			t.Errorf("normalizeHostToken(%q) = %q; expected %q", tt.token, got, tt.expected)
		}
	}
}

func TestWildcardPatternToRegex(t *testing.T) {
	regex := wildcardPatternToRegex("*.Example.com")

	if regex != "(?i)^.*\\.Example\\.com$" {
		t.Fatalf("unexpected regex: %q", regex)
	}

	if matched, _ := regexp.MatchString(regex, "api.example.com"); !matched {
		t.Fatal("expected wildcard regex to match subdomain")
	}

	if matched, _ := regexp.MatchString(regex, "API.EXAMPLE.COM"); !matched {
		t.Fatal("expected wildcard regex to match case-insensitively")
	}

	if matched, _ := regexp.MatchString(regex, "example.com"); matched {
		t.Fatal("expected wildcard regex not to match apex domain")
	}
}

func TestGetEnvDuration(t *testing.T) {
	const key = "DYNAMIC_PROXY_TEST_DURATION"
	defaultVal := 9 * time.Second

	t.Setenv(key, "25s")
	if got := GetEnvDuration(key, defaultVal); got != 25*time.Second {
		t.Fatalf("GetEnvDuration valid = %v; expected %v", got, 25*time.Second)
	}

	t.Setenv(key, "invalid")
	if got := GetEnvDuration(key, defaultVal); got != defaultVal {
		t.Fatalf("GetEnvDuration invalid = %v; expected default %v", got, defaultVal)
	}

	t.Setenv(key, "0s")
	if got := GetEnvDuration(key, defaultVal); got != defaultVal {
		t.Fatalf("GetEnvDuration zero = %v; expected default %v", got, defaultVal)
	}
}

func TestGetEnvInt(t *testing.T) {
	const key = "DYNAMIC_PROXY_TEST_INT"
	defaultVal := 123

	t.Setenv(key, "456")
	if got := GetEnvInt(key, defaultVal); got != 456 {
		t.Fatalf("GetEnvInt valid = %d; expected %d", got, 456)
	}

	t.Setenv(key, "oops")
	if got := GetEnvInt(key, defaultVal); got != defaultVal {
		t.Fatalf("GetEnvInt invalid = %d; expected default %d", got, defaultVal)
	}

	t.Setenv(key, "0")
	if got := GetEnvInt(key, defaultVal); got != defaultVal {
		t.Fatalf("GetEnvInt zero = %d; expected default %d", got, defaultVal)
	}
}

func TestLoadConfigTimeoutOverrides(t *testing.T) {
	// Keep this explicit so we can assert each env variable wiring.
	t.Setenv("SERVER_READ_HEADER_TIMEOUT", "11s")
	t.Setenv("SERVER_READ_TIMEOUT", "22s")
	t.Setenv("SERVER_WRITE_TIMEOUT", "33s")
	t.Setenv("SERVER_IDLE_TIMEOUT", "44s")
	t.Setenv("SERVER_MAX_HEADER_BYTES", "2048")
	t.Setenv("CLIENT_REQUEST_TIMEOUT", "55s")
	t.Setenv("TRANSPORT_DIAL_TIMEOUT", "6s")
	t.Setenv("TRANSPORT_KEEP_ALIVE", "7s")
	t.Setenv("TRANSPORT_TLS_HANDSHAKE_TIMEOUT", "8s")
	t.Setenv("TRANSPORT_RESPONSE_HEADER_TIMEOUT", "9s")
	t.Setenv("TRANSPORT_EXPECT_CONTINUE_TIMEOUT", "10s")
	t.Setenv("TRANSPORT_IDLE_CONN_TIMEOUT", "12s")
	t.Setenv("TUNNEL_CONNECT_READ_WRITE_TIMEOUT", "13s")

	cfg := LoadConfig()

	if cfg.ServerReadHeaderTimeout != 11*time.Second {
		t.Fatalf("ServerReadHeaderTimeout = %v", cfg.ServerReadHeaderTimeout)
	}
	if cfg.ServerReadTimeout != 22*time.Second {
		t.Fatalf("ServerReadTimeout = %v", cfg.ServerReadTimeout)
	}
	if cfg.ServerWriteTimeout != 33*time.Second {
		t.Fatalf("ServerWriteTimeout = %v", cfg.ServerWriteTimeout)
	}
	if cfg.ServerIdleTimeout != 44*time.Second {
		t.Fatalf("ServerIdleTimeout = %v", cfg.ServerIdleTimeout)
	}
	if cfg.ServerMaxHeaderBytes != 2048 {
		t.Fatalf("ServerMaxHeaderBytes = %d", cfg.ServerMaxHeaderBytes)
	}
	if cfg.ClientRequestTimeout != 55*time.Second {
		t.Fatalf("ClientRequestTimeout = %v", cfg.ClientRequestTimeout)
	}
	if cfg.TransportDialTimeout != 6*time.Second {
		t.Fatalf("TransportDialTimeout = %v", cfg.TransportDialTimeout)
	}
	if cfg.TransportKeepAlive != 7*time.Second {
		t.Fatalf("TransportKeepAlive = %v", cfg.TransportKeepAlive)
	}
	if cfg.TransportTLSHandshakeTimeout != 8*time.Second {
		t.Fatalf("TransportTLSHandshakeTimeout = %v", cfg.TransportTLSHandshakeTimeout)
	}
	if cfg.TransportResponseHeaderTimeout != 9*time.Second {
		t.Fatalf("TransportResponseHeaderTimeout = %v", cfg.TransportResponseHeaderTimeout)
	}
	if cfg.TransportExpectContinueTimeout != 10*time.Second {
		t.Fatalf("TransportExpectContinueTimeout = %v", cfg.TransportExpectContinueTimeout)
	}
	if cfg.TransportIdleConnTimeout != 12*time.Second {
		t.Fatalf("TransportIdleConnTimeout = %v", cfg.TransportIdleConnTimeout)
	}
	if cfg.TunnelConnectReadWriteTimeout != 13*time.Second {
		t.Fatalf("TunnelConnectReadWriteTimeout = %v", cfg.TunnelConnectReadWriteTimeout)
	}
}

func TestLoadConfigTimeoutDefaults(t *testing.T) {
	clearEnv(t,
		"SERVER_READ_HEADER_TIMEOUT",
		"SERVER_READ_TIMEOUT",
		"SERVER_WRITE_TIMEOUT",
		"SERVER_IDLE_TIMEOUT",
		"SERVER_MAX_HEADER_BYTES",
		"CLIENT_REQUEST_TIMEOUT",
		"TRANSPORT_DIAL_TIMEOUT",
		"TRANSPORT_KEEP_ALIVE",
		"TRANSPORT_TLS_HANDSHAKE_TIMEOUT",
		"TRANSPORT_RESPONSE_HEADER_TIMEOUT",
		"TRANSPORT_EXPECT_CONTINUE_TIMEOUT",
		"TRANSPORT_IDLE_CONN_TIMEOUT",
		"TUNNEL_CONNECT_READ_WRITE_TIMEOUT",
	)

	cfg := LoadConfig()

	if cfg.ServerReadHeaderTimeout != defaultServerReadHeaderTimeout {
		t.Fatalf("ServerReadHeaderTimeout default = %v", cfg.ServerReadHeaderTimeout)
	}
	if cfg.ServerReadTimeout != defaultServerReadTimeout {
		t.Fatalf("ServerReadTimeout default = %v", cfg.ServerReadTimeout)
	}
	if cfg.ServerWriteTimeout != defaultServerWriteTimeout {
		t.Fatalf("ServerWriteTimeout default = %v", cfg.ServerWriteTimeout)
	}
	if cfg.ServerIdleTimeout != defaultServerIdleTimeout {
		t.Fatalf("ServerIdleTimeout default = %v", cfg.ServerIdleTimeout)
	}
	if cfg.ServerMaxHeaderBytes != defaultServerMaxHeaderBytes {
		t.Fatalf("ServerMaxHeaderBytes default = %d", cfg.ServerMaxHeaderBytes)
	}
	if cfg.ClientRequestTimeout != defaultClientRequestTimeout {
		t.Fatalf("ClientRequestTimeout default = %v", cfg.ClientRequestTimeout)
	}
	if cfg.TransportDialTimeout != defaultTransportDialTimeout {
		t.Fatalf("TransportDialTimeout default = %v", cfg.TransportDialTimeout)
	}
	if cfg.TransportKeepAlive != defaultTransportKeepAlive {
		t.Fatalf("TransportKeepAlive default = %v", cfg.TransportKeepAlive)
	}
	if cfg.TransportTLSHandshakeTimeout != defaultTransportTLSHandshakeTimeout {
		t.Fatalf("TransportTLSHandshakeTimeout default = %v", cfg.TransportTLSHandshakeTimeout)
	}
	if cfg.TransportResponseHeaderTimeout != defaultTransportResponseHeaderTimeout {
		t.Fatalf("TransportResponseHeaderTimeout default = %v", cfg.TransportResponseHeaderTimeout)
	}
	if cfg.TransportExpectContinueTimeout != defaultTransportExpectContinueTimeout {
		t.Fatalf("TransportExpectContinueTimeout default = %v", cfg.TransportExpectContinueTimeout)
	}
	if cfg.TransportIdleConnTimeout != defaultTransportIdleConnTimeout {
		t.Fatalf("TransportIdleConnTimeout default = %v", cfg.TransportIdleConnTimeout)
	}
	if cfg.TunnelConnectReadWriteTimeout != defaultTunnelConnectReadWriteTimeout {
		t.Fatalf("TunnelConnectReadWriteTimeout default = %v", cfg.TunnelConnectReadWriteTimeout)
	}
}

func clearEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("failed to unset env %q: %v", key, err)
		}
	}
}
