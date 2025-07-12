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
	"regexp"
	"strings"

	"DynamicProxy/internal/config"

	"github.com/Azure/go-ntlmssp"
)

func Start(cfg config.Config) error {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Request %s %s\n", r.Method, r.Host)
		HandleRequest(w, r, cfg)
	})
	log.Printf("Starting proxy on %s (upstream=%s, auth=%s, exceptions=%v)\n",
		cfg.ListenAddr, cfg.UpstreamProxy, cfg.ProxyAuth, cfg.ProxyExceptions,
	)
	return http.ListenAndServe(cfg.ListenAddr, handler)
}

func HandleRequest(w http.ResponseWriter, req *http.Request, cfg config.Config) {
	if req.Method == http.MethodConnect {
		HandleHttps(w, req, cfg)
	} else {
		HandleHttp(w, req, cfg)
	}
}

func HandleHttps(w http.ResponseWriter, req *http.Request, cfg config.Config) {
	useUpstream := !Bypass(req.Host, cfg.ProxyExceptions)
	EstablishTunnel(w, req, cfg, useUpstream)
}

func HandleHttp(w http.ResponseWriter, req *http.Request, cfg config.Config) {
	var transport http.RoundTripper
	if Bypass(req.Host, cfg.ProxyExceptions) {
		transport = http.DefaultTransport
	} else {
		transport = NewUpstreamTransport(cfg)
	}
	ProxyRequest(w, req, transport)
}

func ProxyRequest(w http.ResponseWriter, req *http.Request, transport http.RoundTripper) {
	client := &http.Client{Transport: transport}
	outbound := CloneRequest(req)
	resp, err := client.Do(outbound)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	CopyResponse(w, resp)
}

func Bypass(host string, exceptions []string) bool {
	host = StripPort(host)

	for _, pattern := range exceptions {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}

		if strings.Contains(pattern, "*") {
			regex := WildcardToRegex(pattern)
			if matched, err := regexp.MatchString(regex, host); err == nil && matched {
				return true
			}
		} else {
			if strings.EqualFold(host, pattern) {
				return true
			}
		}
	}
	return false
}

func StripPort(host string) string {
	h, _, err := net.SplitHostPort(host)
	if err == nil {
		return h
	}
	return host
}

func WildcardToRegex(pattern string) string {
	// Escape dots, replace '*' with '.*'
	re := regexp.QuoteMeta(pattern)
	re = strings.ReplaceAll(re, `\*`, ".*")
	return "^" + re + "$"
}

func NewUpstreamTransport(cfg config.Config) http.RoundTripper {
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

func EstablishTunnel(w http.ResponseWriter, req *http.Request, cfg config.Config, useUpstream bool) {
	var backend net.Conn
	var err error

	if useUpstream {
		backend, err = DialViaUpstream(cfg.UpstreamProxy, req.Host)
	} else {
		backend, err = net.Dial("tcp", req.Host)
	}
	if err != nil {
		http.Error(w, "Tunnel connection failed: "+err.Error(), http.StatusServiceUnavailable)
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		backend.Close()
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hj.Hijack()
	if err != nil {
		backend.Close()
		http.Error(w, "Hijack failed: "+err.Error(), http.StatusServiceUnavailable)
		return
	}

	_, _ = fmt.Fprint(clientConn, "HTTP/1.1 200 Connection Established\r\n\r\n")
	Pipe(clientConn, backend)
}

func DialViaUpstream(proxyAddr, target string) (net.Conn, error) {
	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("upstream dial failed: %w", err)
	}

	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	if _, err := conn.Write([]byte(connectReq)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to send CONNECT: %w", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("bad CONNECT response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, fmt.Errorf("upstream CONNECT failed: %s", resp.Status)
	}

	return conn, nil
}

func CloneRequest(req *http.Request) *http.Request {
	outbound := req.Clone(req.Context())
	outbound.RequestURI = ""
	if outbound.URL.Scheme == "" {
		outbound.URL.Scheme = "http"
	}
	outbound.URL.Host = req.Host
	return outbound
}

func CopyResponse(w http.ResponseWriter, resp *http.Response) {
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func Pipe(a, b net.Conn) {
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
