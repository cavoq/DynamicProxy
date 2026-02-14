package config

import (
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	UpstreamProxy   string
	ProxyExceptions []string
	ListenAddr      string
	ProxyAuth       string

	ServerReadHeaderTimeout      time.Duration
	ServerReadTimeout            time.Duration
	ServerWriteTimeout           time.Duration
	ServerIdleTimeout            time.Duration
	ServerMaxHeaderBytes         int
	ClientRequestTimeout         time.Duration
	TransportDialTimeout         time.Duration
	TransportKeepAlive           time.Duration
	TransportTLSHandshakeTimeout time.Duration
	TransportResponseHeaderTimeout time.Duration
	TransportExpectContinueTimeout time.Duration
	TransportIdleConnTimeout     time.Duration
	TunnelConnectReadWriteTimeout time.Duration
}

const (
	defaultServerReadHeaderTimeout      = 10 * time.Second
	defaultServerReadTimeout            = 30 * time.Second
	defaultServerWriteTimeout           = 30 * time.Second
	defaultServerIdleTimeout            = 120 * time.Second
	defaultServerMaxHeaderBytes         = 1 << 20 // 1 MiB
	defaultClientRequestTimeout         = 60 * time.Second
	defaultTransportDialTimeout         = 10 * time.Second
	defaultTransportKeepAlive           = 30 * time.Second
	defaultTransportTLSHandshakeTimeout = 10 * time.Second
	defaultTransportResponseHeaderTimeout = 30 * time.Second
	defaultTransportExpectContinueTimeout = 1 * time.Second
	defaultTransportIdleConnTimeout     = 90 * time.Second
	defaultTunnelConnectReadWriteTimeout = 15 * time.Second
)

func LoadConfig() Config {
	config := Config{
		UpstreamProxy:   GetEnv("UPSTREAM_PROXY", ""),
		ProxyExceptions: []string{},
		ListenAddr:      GetEnv("LISTEN_ADDR", ":8080"),
		ProxyAuth:       GetEnv("PROXY_AUTH", ""),
		ServerReadHeaderTimeout:      GetEnvDuration("SERVER_READ_HEADER_TIMEOUT", defaultServerReadHeaderTimeout),
		ServerReadTimeout:            GetEnvDuration("SERVER_READ_TIMEOUT", defaultServerReadTimeout),
		ServerWriteTimeout:           GetEnvDuration("SERVER_WRITE_TIMEOUT", defaultServerWriteTimeout),
		ServerIdleTimeout:            GetEnvDuration("SERVER_IDLE_TIMEOUT", defaultServerIdleTimeout),
		ServerMaxHeaderBytes:         GetEnvInt("SERVER_MAX_HEADER_BYTES", defaultServerMaxHeaderBytes),
		ClientRequestTimeout:         GetEnvDuration("CLIENT_REQUEST_TIMEOUT", defaultClientRequestTimeout),
		TransportDialTimeout:         GetEnvDuration("TRANSPORT_DIAL_TIMEOUT", defaultTransportDialTimeout),
		TransportKeepAlive:           GetEnvDuration("TRANSPORT_KEEP_ALIVE", defaultTransportKeepAlive),
		TransportTLSHandshakeTimeout: GetEnvDuration("TRANSPORT_TLS_HANDSHAKE_TIMEOUT", defaultTransportTLSHandshakeTimeout),
		TransportResponseHeaderTimeout: GetEnvDuration("TRANSPORT_RESPONSE_HEADER_TIMEOUT", defaultTransportResponseHeaderTimeout),
		TransportExpectContinueTimeout: GetEnvDuration("TRANSPORT_EXPECT_CONTINUE_TIMEOUT", defaultTransportExpectContinueTimeout),
		TransportIdleConnTimeout:     GetEnvDuration("TRANSPORT_IDLE_CONN_TIMEOUT", defaultTransportIdleConnTimeout),
		TunnelConnectReadWriteTimeout: GetEnvDuration("TUNNEL_CONNECT_READ_WRITE_TIMEOUT", defaultTunnelConnectReadWriteTimeout),
	}

	if exceptions := os.Getenv("PROXY_EXCEPTIONS"); exceptions != "" {
		config.ProxyExceptions = GetExceptions(exceptions)
	}

	return config
}

func GetEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

func GetEnvDuration(key string, defaultVal time.Duration) time.Duration {
	val, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(val) == "" {
		return defaultVal
	}
	parsed, err := time.ParseDuration(val)
	if err != nil || parsed <= 0 {
		return defaultVal
	}
	return parsed
}

func GetEnvInt(key string, defaultVal int) int {
	val, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(val) == "" {
		return defaultVal
	}
	parsed, err := strconv.Atoi(val)
	if err != nil || parsed <= 0 {
		return defaultVal
	}
	return parsed
}

func GetExceptions(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var list []string
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}

		if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
			if u, err := url.Parse(p); err == nil && u.Host != "" {
				p = u.Host
			}
		}

		p = strings.TrimSuffix(p, "/")
		list = append(list, p)
	}
	return list
}

func IsException(host string, exceptions []string) bool {
	hostCandidates := buildHostCandidates(host)
	if len(hostCandidates) == 0 {
		return false
	}

	for _, pattern := range exceptions {
		pattern = strings.TrimSpace(strings.TrimSuffix(pattern, "/"))
		if pattern == "" {
			continue
		}

		if strings.Contains(pattern, "*") {
			regex := wildcardPatternToRegex(pattern)
			for _, candidate := range hostCandidates {
				if matched, err := regexp.MatchString(regex, candidate); err == nil && matched {
					return true
				}
			}
			continue
		}

		normalizedPattern := normalizeHostToken(pattern)
		for _, candidate := range hostCandidates {
			if strings.EqualFold(normalizeHostToken(candidate), normalizedPattern) {
				return true
			}
		}
	}

	return false
}

func buildHostCandidates(host string) []string {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil
	}

	candidates := []string{host}
	if withoutPort, ok := splitOutPort(host); ok && !strings.EqualFold(withoutPort, host) {
		candidates = append(candidates, withoutPort)
	}
	return candidates
}

func splitOutPort(host string) (string, bool) {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		return host, false
	}
	return h, true
}

func normalizeHostToken(token string) string {
	token = strings.TrimSpace(token)
	if strings.HasPrefix(token, "[") && strings.HasSuffix(token, "]") && len(token) > 2 {
		return token[1 : len(token)-1]
	}
	return token
}

func wildcardPatternToRegex(pattern string) string {
	re := regexp.QuoteMeta(pattern)
	re = strings.ReplaceAll(re, `\*`, ".*")
	return "(?i)^" + re + "$"
}
