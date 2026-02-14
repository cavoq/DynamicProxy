package integration_test

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
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
		_, _ = w.Write(bytes.Clone([]byte(content)))
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(func() { srv.Close() })

	return &testServer{Server: srv, content: content}
}

func newTLSTestServer(t *testing.T, content string) *testServer {
	t.Helper()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Clone([]byte(content)))
	})

	srv := httptest.NewTLSServer(handler)
	t.Cleanup(func() { srv.Close() })

	return &testServer{Server: srv, content: content}
}

type upstreamProxy struct {
	*httptest.Server
	httpRequests    int64
	connectRequests int64
}

func newUpstreamProxy(t *testing.T) *upstreamProxy {
	t.Helper()
	upstream := &upstreamProxy{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			upstream.handleConnect(w, r)
			return
		}

		atomic.AddInt64(&upstream.httpRequests, 1)
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

	srv := httptest.NewServer(handler)
	upstream.Server = srv
	t.Cleanup(func() { srv.Close() })
	return upstream
}

func (u *upstreamProxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&u.connectRequests, 1)

	targetConn, err := net.DialTimeout("tcp", r.Host, 5*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		targetConn.Close()
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		targetConn.Close()
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	pipeConns(clientConn, targetConn)
}

func pipeConns(a, b net.Conn) {
	go func() {
		defer a.Close()
		defer b.Close()
		_, _ = io.Copy(a, b)
	}()
	go func() {
		defer a.Close()
		defer b.Close()
		_, _ = io.Copy(b, a)
	}()
}

func (u *upstreamProxy) HTTPRequests() int64 {
	return atomic.LoadInt64(&u.httpRequests)
}

func (u *upstreamProxy) ConnectRequests() int64 {
	return atomic.LoadInt64(&u.connectRequests)
}

type dynamicProxy struct {
	*exec.Cmd
}

func startDynamicProxy(t *testing.T, upstreamURL, exceptions string) *dynamicProxy {
	t.Helper()

	binaryName := "dynamicproxy"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(".", binaryName)
	absBinaryPath, err := filepath.Abs(binaryPath)
	if err != nil {
		t.Fatalf("failed to resolve binary path: %v", err)
	}

	if _, err := os.Stat(absBinaryPath); err != nil {
		build := exec.Command("go", "build", "-o", binaryPath, "./cmd/main.go")
		build.Stdout = os.Stdout
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			t.Fatalf("failed to build dynamicproxy: %v", err)
		}
	}

	cmd := exec.Command(absBinaryPath)
	cmd.Env = append(os.Environ(),
		"UPSTREAM_PROXY="+upstreamURL,
		"PROXY_EXCEPTIONS="+exceptions,
		"LISTEN_ADDR=127.0.0.1:8080",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start dynamicproxy: %v", err)
	}

	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	if err := waitForPort("127.0.0.1:8080", 5*time.Second); err != nil {
		t.Fatal("dynamicproxy failed to start:", err)
	}

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

func proxyClient(t *testing.T, insecureTLS bool) *http.Client {
	t.Helper()
	proxyURL, _ := url.Parse("http://127.0.0.1:8080")
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecureTLS, // test-only
			},
		},
		Timeout: 10 * time.Second,
	}
}

func TestDynamicProxy_IntegrationHTTP(t *testing.T) {
	upstream := newUpstreamProxy(t)
	directServer := newTestServer(t, "Hello from DIRECT server (bypassed)")
	viaProxyServer := newTestServer(t, "Hello from VIA UPSTREAM server")

	startDynamicProxy(
		t,
		strings.TrimPrefix(upstream.URL, "http://"),
		strings.TrimPrefix(directServer.URL, "http://"),
	)

	client := proxyClient(t, false)

	tests := []struct {
		name     string
		target   string
		expected string
		viaProxy bool
	}{
		{
			name:     "bypass_upstream_http",
			target:   directServer.URL,
			expected: "Hello from DIRECT server (bypassed)",
			viaProxy: false,
		},
		{
			name:     "route_via_upstream_http",
			target:   viaProxyServer.URL,
			expected: "Hello from VIA UPSTREAM server",
			viaProxy: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beforeHTTP := upstream.HTTPRequests()

			resp, err := client.Get(tt.target)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			got := strings.TrimSpace(string(body))

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected status 200, got %d", resp.StatusCode)
			}
			if got != tt.expected {
				t.Fatalf("expected body %q, got %q", tt.expected, got)
			}

			afterHTTP := upstream.HTTPRequests()
			if tt.viaProxy && afterHTTP <= beforeHTTP {
				t.Fatalf("expected upstream HTTP request count to increase (before=%d, after=%d)", beforeHTTP, afterHTTP)
			}
			if !tt.viaProxy && afterHTTP != beforeHTTP {
				t.Fatalf("expected bypass request to skip upstream HTTP proxy (before=%d, after=%d)", beforeHTTP, afterHTTP)
			}
		})
	}
}

func TestDynamicProxy_IntegrationHTTPSConnect(t *testing.T) {
	upstream := newUpstreamProxy(t)
	directTLSServer := newTLSTestServer(t, "Hello from DIRECT TLS server (bypassed)")
	viaProxyTLSServer := newTLSTestServer(t, "Hello from VIA UPSTREAM TLS server")

	startDynamicProxy(
		t,
		strings.TrimPrefix(upstream.URL, "http://"),
		strings.TrimPrefix(directTLSServer.URL, "https://"),
	)

	client := proxyClient(t, true)

	tests := []struct {
		name     string
		target   string
		expected string
		viaProxy bool
	}{
		{
			name:     "bypass_upstream_https_connect",
			target:   directTLSServer.URL,
			expected: "Hello from DIRECT TLS server (bypassed)",
			viaProxy: false,
		},
		{
			name:     "route_via_upstream_https_connect",
			target:   viaProxyTLSServer.URL,
			expected: "Hello from VIA UPSTREAM TLS server",
			viaProxy: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beforeCONNECT := upstream.ConnectRequests()

			resp, err := client.Get(tt.target)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			got := strings.TrimSpace(string(body))

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected status 200, got %d", resp.StatusCode)
			}
			if got != tt.expected {
				t.Fatalf("expected body %q, got %q", tt.expected, got)
			}

			afterCONNECT := upstream.ConnectRequests()
			if tt.viaProxy && afterCONNECT <= beforeCONNECT {
				t.Fatalf("expected upstream CONNECT count to increase (before=%d, after=%d)", beforeCONNECT, afterCONNECT)
			}
			if !tt.viaProxy && afterCONNECT != beforeCONNECT {
				t.Fatalf("expected bypass CONNECT to skip upstream proxy (before=%d, after=%d)", beforeCONNECT, afterCONNECT)
			}
		})
	}
}
