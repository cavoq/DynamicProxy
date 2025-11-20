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
	"os"
	"regexp"
	"strings"

	"github.com/cavoq/DynamicProxy/internal/config"

	"github.com/Azure/go-ntlmssp"
)

var (
	Info  = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime|log.Lmsgprefix)
	Warn  = log.New(os.Stdout, "WARN: ", log.Ldate|log.Ltime|log.Lmsgprefix)
	Error = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lmsgprefix)
)

func Start(cfg config.Config) error {
	Info.Printf("Starting proxy on %s (upstream=%s, auth=%s, exceptions=%v)",
		cfg.ListenAddr, cfg.UpstreamProxy, cfg.ProxyAuth, cfg.ProxyExceptions)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleRequest(w, r, cfg)
	})
	return http.ListenAndServe(cfg.ListenAddr, handler)
}

func HandleRequest(w http.ResponseWriter, req *http.Request, cfg config.Config) {
	Info.Printf("Processing request %s %s", req.Method, req.Host)
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
		Error.Printf("ProxyRequest error for %s %s: %v", req.Method, req.Host, err)
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
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
			} else if err != nil {
				Warn.Printf("Invalid pattern %q in ProxyExceptions: %v", pattern, err)
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
	re := regexp.QuoteMeta(pattern)
	re = strings.ReplaceAll(re, `\*`, ".*")
	return "^" + re + "$"
}

func NewUpstreamTransport(cfg config.Config) http.RoundTripper {
	url, err := url.Parse("http://" + cfg.UpstreamProxy)
	if err != nil {
		Error.Printf("Invalid upstream proxy url %q: %v", cfg.UpstreamProxy, err)
		return http.DefaultTransport
	}
	base := &http.Transport{
		Proxy: http.ProxyURL(url),
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
		Error.Printf("Tunnel connection failed to %s: %v", req.Host, err)
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		backend.Close()
		Error.Println("HTTP Hijacking not supported")
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hj.Hijack()
	if err != nil {
		backend.Close()
		Error.Printf("Hijack failed for %s: %v", req.Host, err)
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
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
	_, err := io.Copy(w, resp.Body)
	if err != nil {
		Error.Printf("Error copying response body: %v", err)
	}
}

func Pipe(a, b net.Conn) {
	go func() {
		defer a.Close()
		defer b.Close()
		if _, err := io.Copy(a, b); err != nil {
			Warn.Printf("Pipe error (a->b): %v", err)
		}
	}()
	go func() {
		defer a.Close()
		defer b.Close()
		if _, err := io.Copy(b, a); err != nil {
			Warn.Printf("Pipe error (b->a): %v", err)
		}
	}()
}
