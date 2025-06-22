package proxy

import (
	"DynamicProxy/internal/config"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

func Start(cfg config.Config) error {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleRequest(w, r, cfg)
	})
	log.Printf("Starting proxy on %s\n", cfg.ListenAddr)
	return http.ListenAndServe(cfg.ListenAddr, handler)
}

func handleRequest(w http.ResponseWriter, req *http.Request, cfg config.Config) {
	hostPort := req.URL.Host
	log.Printf("Request for %s %s\n", req.Method, hostPort)

	if shouldBypassProxy(hostPort, cfg.ProxyExceptions) {
		proxyDirect(w, req)
	} else if cfg.UpstreamProxy != "" {
		proxyToUpstream(w, req, cfg.UpstreamProxy)
	} else {
		proxyDirect(w, req)
	}
}

func shouldBypassProxy(hostPort string, exceptions []string) bool {
	for _, ex := range exceptions {
		if strings.EqualFold(hostPort, ex) {
			return true
		}
	}
	return false
}

func proxyToUpstream(w http.ResponseWriter, req *http.Request, upstream string) {
	proxyURL, err := url.Parse("http://" + upstream)
	if err != nil {
		http.Error(w, "Invalid upstream proxy: "+err.Error(), http.StatusBadGateway)
		return
	}
	transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	copyResponse(w, resp)
}

func proxyDirect(w http.ResponseWriter, req *http.Request) {
	transport := http.DefaultTransport
	resp, err := transport.RoundTrip(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	copyResponse(w, resp)
}

func copyResponse(w http.ResponseWriter, resp *http.Response) {
	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
