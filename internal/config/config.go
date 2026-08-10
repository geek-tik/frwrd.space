package config

import (
	"fmt"
	"os"
)

type Server struct {
	HTTPAddr      string
	EdgeAddr      string
	AgentAddr     string
	BaseDomain    string
	DatabaseURL   string
	JWTSecret     string
	SecureCookies bool
}

func LoadServer() (Server, error) {
	cfg := Server{
		HTTPAddr:      envOr("HTTP_ADDR", ":8080"),
		EdgeAddr:      envOr("EDGE_ADDR", ":8081"),
		AgentAddr:     envOr("AGENT_ADDR", ":8082"),
		BaseDomain:    envOr("BASE_DOMAIN", "frwrd.space"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		JWTSecret:     os.Getenv("JWT_SECRET"),
		SecureCookies: envOr("SECURE_COOKIES", "false") == "true",
	}

	if cfg.DatabaseURL == "" {
		return cfg, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return cfg, fmt.Errorf("JWT_SECRET is required")
	}

	return cfg, nil
}

type Agent struct {
	ServerURL    string
	Token        string
	LocalHost    string
	DashboardURL string
}

func LoadAgent() Agent {
	return Agent{
		ServerURL:    envOr("FORWARD_SERVER_URL", "ws://server:8082/agent/connect"),
		Token:        os.Getenv("FORWARD_API_TOKEN"),
		LocalHost:    envOr("FORWARD_LOCAL_HOST", "host.docker.internal"),
		DashboardURL: envOr("FORWARD_DASHBOARD_URL", "http://localhost:3080"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
