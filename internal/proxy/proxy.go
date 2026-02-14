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
	"strings"
	"time"

	"github.com/cavoq/DynamicProxy/internal/config"

	"github.com/Azure/go-ntlmssp"
)

var (
	Info  = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime|log.Lmsgprefix)
	Warn  = log.New(os.Stdout, "WARN: ", log.Ldate|log.Ltime|log.Lmsgprefix)
	Error = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lmsgprefix)
)

type requestTransports struct {
	direct   http.RoundTripper
	upstream http.RoundTripper
}

func Start(cfg config.Config) error {
	Info.Printf("Starting proxy on %s (upstream=%s, auth=%s, exceptions=%v)",
		cfg.ListenAddr, cfg.UpstreamProxy, cfg.ProxyAuth, cfg.ProxyExceptions)
	transports := requestTransports{
		direct:   NewDirectTransport(cfg),
		upstream: NewUpstreamTransport(cfg),
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleRequestWithTransports(w, r, cfg, transports)
	})
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ServerReadHeaderTimeout,
		ReadTimeout:       cfg.ServerReadTimeout,
		WriteTimeout:      cfg.ServerWriteTimeout,
		IdleTimeout:       cfg.ServerIdleTimeout,
		MaxHeaderBytes:    cfg.ServerMaxHeaderBytes,
	}
	return server.ListenAndServe()
}

func HandleRequest(w http.ResponseWriter, req *http.Request, cfg config.Config) {
	transports := requestTransports{
		direct:   NewDirectTransport(cfg),
		upstream: NewUpstreamTransport(cfg),
	}
	handleRequestWithTransports(w, req, cfg, transports)
}

func handleRequestWithTransports(w http.ResponseWriter, req *http.Request, cfg config.Config, transports requestTransports) {
	Info.Printf("Processing request %s %s", req.Method, req.Host)
	if req.Method == http.MethodConnect {
		HandleHttps(w, req, cfg)
	} else {
		handleHttpWithTransports(w, req, cfg, transports)
	}
}

func HandleHttps(w http.ResponseWriter, req *http.Request, cfg config.Config) {
	useUpstream := !Bypass(req.Host, cfg.ProxyExceptions)
	EstablishTunnel(w, req, cfg, useUpstream)
}

func HandleHttp(w http.ResponseWriter, req *http.Request, cfg config.Config) {
	transports := requestTransports{
		direct:   NewDirectTransport(cfg),
		upstream: NewUpstreamTransport(cfg),
	}
	handleHttpWithTransports(w, req, cfg, transports)
}

func handleHttpWithTransports(w http.ResponseWriter, req *http.Request, cfg config.Config, transports requestTransports) {
	var transport http.RoundTripper
	if Bypass(req.Host, cfg.ProxyExceptions) {
		transport = transports.direct
	} else {
		transport = transports.upstream
	}
	ProxyRequest(w, req, transport, cfg)
}

func ProxyRequest(w http.ResponseWriter, req *http.Request, transport http.RoundTripper, cfg config.Config) {
	client := &http.Client{
		Transport: transport,
		Timeout:   cfg.ClientRequestTimeout,
	}
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
	return config.IsException(host, exceptions)
}

func NewDirectTransport(cfg config.Config) http.RoundTripper {
	return newTransport(cfg, nil)
}

func NewUpstreamTransport(cfg config.Config) http.RoundTripper {
	proxyURL, err := url.Parse("http://" + cfg.UpstreamProxy)
	if err != nil {
		Error.Printf("Invalid upstream proxy url %q: %v", cfg.UpstreamProxy, err)
		return NewDirectTransport(cfg)
	}
	base := newTransport(cfg, proxyURL)
	if strings.EqualFold(cfg.ProxyAuth, "ntlm") {
		base.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{} // Disable HTTP/2 for NTLM
		return ntlmssp.Negotiator{RoundTripper: base}
	}
	return base
}

func newTransport(cfg config.Config, proxyURL *url.URL) *http.Transport {
	dialer := &net.Dialer{
		Timeout:   cfg.TransportDialTimeout,
		KeepAlive: cfg.TransportKeepAlive,
	}
	tr := &http.Transport{
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   cfg.TransportTLSHandshakeTimeout,
		ResponseHeaderTimeout: cfg.TransportResponseHeaderTimeout,
		ExpectContinueTimeout: cfg.TransportExpectContinueTimeout,
		IdleConnTimeout:       cfg.TransportIdleConnTimeout,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
	}
	if proxyURL != nil {
		tr.Proxy = http.ProxyURL(proxyURL)
	}
	return tr
}

func EstablishTunnel(w http.ResponseWriter, req *http.Request, cfg config.Config, useUpstream bool) {
	var backend net.Conn
	var err error

	if useUpstream {
		backend, err = DialViaUpstream(cfg.UpstreamProxy, req.Host, cfg)
	} else {
		backend, err = net.DialTimeout("tcp", req.Host, cfg.TransportDialTimeout)
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

func DialViaUpstream(proxyAddr, target string, cfg config.Config) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", proxyAddr, cfg.TransportDialTimeout)
	if err != nil {
		return nil, fmt.Errorf("upstream dial failed: %w", err)
	}
	if err := conn.SetDeadline(time.Now().Add(cfg.TunnelConnectReadWriteTimeout)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to set upstream CONNECT deadline: %w", err)
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
	if err := conn.SetDeadline(time.Time{}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to clear upstream CONNECT deadline: %w", err)
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
