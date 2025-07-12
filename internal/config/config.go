package config

import (
	"net/url"
	"os"
	"strings"
)

type Config struct {
	UpstreamProxy   string
	ProxyExceptions []string
	ListenAddr      string
	ProxyAuth       string
}

func LoadConfig() Config {
	config := Config{
		UpstreamProxy:   getEnv("UPSTREAM_PROXY", ""),
		ProxyExceptions: []string{},
		ListenAddr:      getEnv("LISTEN_ADDR", ":8080"),
		ProxyAuth:       getEnv("PROXY_AUTH", ""),
	}

	if exceptions := os.Getenv("PROXY_EXCEPTIONS"); exceptions != "" {
		config.ProxyExceptions = parseList(exceptions)
	}

	return config
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

func parseList(s string) []string {
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

func isException(host string, exceptions []string) bool {
	for _, pattern := range exceptions {
		if after, ok := strings.CutPrefix(pattern, "*."); ok {
			suffix := after
			if strings.HasSuffix(host, suffix) {
				return true
			}
		} else if host == pattern {
			return true
		}
	}
	return false
}
