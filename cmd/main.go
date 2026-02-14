package main

import (
	"log"

	"github.com/cavoq/DynamicProxy/internal/config"
	"github.com/cavoq/DynamicProxy/internal/proxy"
)

func main() {
	cfg := config.LoadConfig()

	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080"
	}

	log.Printf("Upstream Proxy: %s", cfg.UpstreamProxy)
	log.Printf("Proxy Exceptions: %v", cfg.ProxyExceptions)
	log.Printf("Authentication: %s", cfg.ProxyAuth)

	if err := proxy.Start(cfg); err != nil {
		log.Fatalf("Failed to start proxy: %v", err)
	}
}
