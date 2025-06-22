package proxy

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"

	"DynamicProxy/internal/config"

	"github.com/Azure/go-ntlmssp"
)

func Start(cfg config.Config) error {
	addr := cfg.ListenAddr
	if addr == "" {
		addr = ":8080"
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Request %s %s\n", r.Method, r.Host)
		HandleRequest(w, r, cfg)
	})

	log.Printf("Starting proxy on %s (upstream=%s, auth=%s, exceptions=%v)\n",
		addr, cfg.UpstreamProxy, cfg.ProxyAuth, cfg.ProxyExceptions,
	)
	return http.ListenAndServe(addr, handler)
}

func HandleRequest(w http.ResponseWriter, req *http.Request, cfg config.Config) {
	if req.Method == http.MethodConnect {
		handleTunneling(w, req, cfg)
	} else {
		handleHTTP(w, req, cfg)
	}
}

func handleHTTP(w http.ResponseWriter, req *http.Request, cfg config.Config) {
	if shouldBypass(req.Host, cfg.ProxyExceptions) {
		proxyDirect(w, req)
	} else {
		proxyToUpstream(w, req, cfg)
	}
}

func proxyDirect(w http.ResponseWriter, req *http.Request) {
	transport := http.DefaultTransport

	outbound := cloneRequestForClient(req)
	resp, err := transport.RoundTrip(outbound)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	copyResponse(w, resp)
}

func proxyToUpstream(w http.ResponseWriter, req *http.Request, cfg config.Config) {
	client := &http.Client{
		Transport: newUpstreamTransport(cfg),
	}

	outbound := cloneRequestForClient(req)
	resp, err := client.Do(outbound)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	copyResponse(w, resp)
}

func cloneRequestForClient(req *http.Request) *http.Request {
	outbound := req.Clone(req.Context())
	outbound.RequestURI = ""
	if outbound.URL.Scheme == "" {
		outbound.URL.Scheme = "http"
	}
	outbound.URL.Host = req.Host
	return outbound
}

func newUpstreamTransport(cfg config.Config) http.RoundTripper {
	proxyURL, _ := url.Parse("http://" + cfg.UpstreamProxy)

	base := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}

	if strings.EqualFold(cfg.ProxyAuth, "ntlm") {
		base.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{} // Disable HTTP/2 for NTLM
		return ntlmssp.Negotiator{RoundTripper: base}
	}

	return base
}

func handleTunneling(w http.ResponseWriter, req *http.Request, cfg config.Config) {
	dest := req.Host

	if shouldBypass(dest, cfg.ProxyExceptions) {
		tunnel(w, req, dest)
	} else {
		tunnelViaUpstream(w, req, cfg, dest)
	}
}

func tunnel(w http.ResponseWriter, _ *http.Request, dest string) {
	backend, err := net.Dial("tcp", dest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		backend.Close()
		return
	}
	clientConn, _, err := hj.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		backend.Close()
		return
	}

	fmt.Fprintf(clientConn, "HTTP/1.1 200 Connection Established\r\n\r\n")
	pipe(clientConn, backend)
}

func tunnelViaUpstream(w http.ResponseWriter, _ *http.Request, cfg config.Config, dest string) {
	backendConn, err := net.Dial("tcp", cfg.UpstreamProxy)
	if err != nil {
		http.Error(w, "Failed to connect to upstream proxy: "+err.Error(), http.StatusBadGateway)
		return
	}

	fmt.Fprintf(backendConn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", dest, dest)

	br := bufio.NewReader(backendConn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		backendConn.Close()
		http.Error(w, "Failed to read upstream response: "+err.Error(), http.StatusBadGateway)
		return
	}
	if resp.StatusCode != http.StatusOK {
		backendConn.Close()
		http.Error(w, "Upstream proxy CONNECT failed: "+resp.Status, http.StatusBadGateway)
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		backendConn.Close()
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hj.Hijack()
	if err != nil {
		backendConn.Close()
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	fmt.Fprintf(clientConn, "HTTP/1.1 200 Connection Established\r\n\r\n")

	pipe(clientConn, backendConn)
}

func shouldBypass(host string, exceptions []string) bool {
	for _, ex := range exceptions {
		if strings.EqualFold(host, ex) {
			return true
		}
	}
	return false
}

func pipe(a, b net.Conn) {
	go func() {
		defer a.Close()
		defer b.Close()
		io.Copy(a, b)
	}()
	go func() {
		defer a.Close()
		defer b.Close()
		io.Copy(b, a)
	}()
}

func copyResponse(w http.ResponseWriter, resp *http.Response) {
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
