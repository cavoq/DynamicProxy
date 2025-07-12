package config

import (
	"testing"
)

func TestParseList(t *testing.T) {
	input := "http://localhost, *.example.com, api.svc.com, https://www.test.org/"
	expected := []string{"localhost", "*.example.com", "api.svc.com", "www.test.org"}

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
	exceptions := []string{"localhost", "*.example.com", "api.svc.com"}

	tests := []struct {
		host     string
		expected bool
	}{
		{"localhost", true},
		{"www.example.com", true},
		{"example.com", true},
		{"api.example.com", true},
		{"api.svc.com", true},
		{"other.com", false},
		{"svc.api.com", false},
	}

	for _, tt := range tests {
		if IsException(tt.host, exceptions) != tt.expected {
			t.Errorf("IsException(%q) = %v; expected %v", tt.host, !tt.expected, tt.expected)
		}
	}
}
