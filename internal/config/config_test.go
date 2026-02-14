package config

import (
	"reflect"
	"regexp"
	"testing"
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
