package config

import (
	"net"
	"net/url"
	"os"
	"regexp"
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
