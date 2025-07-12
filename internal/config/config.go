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
		UpstreamProxy:   GetEnv("UPSTREAM_PROXY", ""),
		ProxyExceptions: []string{},
		ListenAddr:      GetEnv("LISTEN_ADDR", ":8080"),
		ProxyAuth:       GetEnv("PROXY_AUTH", ""),
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
