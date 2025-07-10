package config

import (
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
		if p != "" {
			list = append(list, p)
		}
	}
	return list
}
