package integration_test

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

type testServer struct {
	*httptest.Server
	content string
}

func newTestServer(t *testing.T, content string) *testServer {
	t.Helper()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("[Dummy Server %s] %s %s", content[:10], r.Method, r.URL.Path)
		_, _ = w.Write(bytes.Clone([]byte(content)))
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(func() {
		t.Logf("Shutting down dummy server: %s", srv.URL)
		srv.Close()
	})

	return &testServer{Server: srv, content: content}
}

func newUpstreamProxy(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Logf("[Upstream Proxy] → %s %s", r.Method, r.URL)

		resp, err := http.Get(r.URL.String())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		maps.Copy(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(func() {
		t.Logf("Shutting down upstream proxy: %s", srv.URL)
		srv.Close()
	})
	return srv
}

type dynamicProxy struct {
	*exec.Cmd
}

func startDynamicProxy(t *testing.T, upstreamURL, exceptions string) *dynamicProxy {
	t.Helper()

	if _, err := exec.LookPath("./dynamicproxy"); err != nil {
		t.Log("Building dynamicproxy...")
		build := exec.Command("go", "build", "-o", "dynamicproxy", "cmd/main.go")
		build.Stdout = os.Stdout
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			t.Fatalf("Failed to build dynamicproxy: %v", err)
		}
	}

	t.Log("Starting dynamicproxy...")
	cmd := exec.Command("./dynamicproxy")
	cmd.Env = append(os.Environ(),
		"UPSTREAM_PROXY="+upstreamURL,
		"PROXY_EXCEPTIONS="+exceptions,
		"LISTEN_ADDR=127.0.0.1:8080",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start dynamicproxy: %v", err)
	}

	t.Cleanup(func() {
		t.Log("Killing dynamicproxy...")
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	if err := waitForPort("127.0.0.1:8080", 5*time.Second); err != nil {
		t.Fatal("dynamicproxy failed to start:", err)
	}

	t.Log("dynamicproxy is ready on :8080")
	return &dynamicProxy{Cmd: cmd}
}

func waitForPort(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", addr)
}

func proxyClient(t *testing.T) *http.Client {
	t.Helper()
	proxyURL, _ := url.Parse("http://127.0.0.1:8080")
	return &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   10 * time.Second,
	}
}

func TestDynamicProxy_Integration(t *testing.T) {
	upstream := newUpstreamProxy(t)
	directServer := newTestServer(t, "Hello from DIRECT server (bypassed)")
	viaProxyServer := newTestServer(t, "Hello from VIA UPSTREAM server")

	startDynamicProxy(
		t,
		strings.TrimPrefix(upstream.URL, "http://"),
		strings.TrimPrefix(directServer.URL, "http://"),
	)

	client := proxyClient(t)

	tests := []struct {
		name     string
		target   string
		expected string
		viaProxy bool
	}{
		{
			name:     "bypass_upstream",
			target:   directServer.URL,
			expected: "Hello from DIRECT server (bypassed)",
			viaProxy: false,
		},
		{
			name:     "route_via_upstream",
			target:   viaProxyServer.URL,
			expected: "Hello from VIA UPSTREAM server",
			viaProxy: true,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp, err := client.Get(tt.target)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			got := strings.TrimSpace(string(body))

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("Expected status 200, got %d", resp.StatusCode)
			}
			if got != tt.expected {
				t.Fatalf("Expected body %q, got %q", tt.expected, got)
			}

			t.Logf("✓ %s - response correct", tt.name)
		})
	}
}
