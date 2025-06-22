package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type mockTransport struct {
	response *http.Response
	err      error
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.response, m.err
}

func TestShouldBypassProxy(t *testing.T) {
	exceptions := []string{"host1:8080", "HOST2:9090"}
	if !shouldBypassProxy("host1:8080", exceptions) {
		t.Error("Expected host1:8080 to bypass proxy")
	}
	if !shouldBypassProxy("host2:9090", exceptions) {
		t.Error("Expected HOST2:9090 to bypass proxy (case-insensitive)")
	}
	if shouldBypassProxy("otherhost:1234", exceptions) {
		t.Error("Did not expect otherhost:1234 to bypass proxy")
	}
}

func TestProxyDirect(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	w := httptest.NewRecorder()

	orig := http.DefaultTransport
	http.DefaultTransport = &mockTransport{
		response: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("OK"))},
		err:      nil,
	}
	defer func() { http.DefaultTransport = orig }()

	proxyDirect(w, req)
	resp := w.Result()
	if resp.StatusCode != 200 {
		t.Errorf("proxyDirect status = %d; want 200", resp.StatusCode)
	}
	body := w.Body.String()
	if strings.TrimSpace(body) != "OK" {
		t.Errorf("proxyDirect body = %q; want 'OK'", body)
	}
}

func TestProxyToUpstream_InvalidURL(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com", nil)
	w := httptest.NewRecorder()
	proxyToUpstream(w, req, ":invalid")
	resp := w.Result()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("Expected 502 for invalid upstream, got %d", resp.StatusCode)
	}
}
