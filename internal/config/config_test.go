package config

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	origUpstream := os.Getenv("UPSTREAM_PROXY")
	origExceptions := os.Getenv("PROXY_EXCEPTIONS")
	defer func() {
		os.Setenv("UPSTREAM_PROXY", origUpstream)
		os.Setenv("PROXY_EXCEPTIONS", origExceptions)
	}()

	os.Setenv("UPSTREAM_PROXY", "upstream.example.com:8080")
	os.Setenv("PROXY_EXCEPTIONS", "host1:1234, host2:5678, host3:9012")

	cfg := LoadConfig()

	if cfg.UpstreamProxy != "upstream.example.com:8080" {
		t.Errorf("UpstreamProxy = %q; want %q", cfg.UpstreamProxy, "upstream.example.com:8080")
	}

	expected := []string{"host1:1234", "host2:5678", "host3:9012"}
	if len(cfg.ProxyExceptions) != len(expected) {
		t.Fatalf("ProxyExceptions length = %d; want %d", len(cfg.ProxyExceptions), len(expected))
	}
	for i := range expected {
		if cfg.ProxyExceptions[i] != expected[i] {
			t.Errorf("ProxyExceptions[%d] = %q; want %q", i, cfg.ProxyExceptions[i], expected[i])
		}
	}
}

func TestParseList_Empty(t *testing.T) {
	got := parseList("")
	if len(got) != 0 {
		t.Errorf("parseList(\"\") = %v; want empty slice", got)
	}
}

func TestParseList_Whitespace(t *testing.T) {
	got := parseList("  ,  , host:1234 ,  , ")
	want := []string{"host:1234"}
	if len(got) != len(want) {
		t.Fatalf("parseList whitespace length = %d; want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("parseList whitespace[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}
