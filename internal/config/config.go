// Package config loads runtime settings from environment variables.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Listen         string        // HTTP listen address, e.g. :8000
	DatabasePath   string        // SQLite file
	JWTSecret      string        // empty = generated and persisted in DB
	TokenTTL       time.Duration // admin session lifetime
	SubPath        string        // public subscription prefix, e.g. /sub
	SubPublicURL   string        // base URL used when rendering subscription links
	SubUpdateHours int           // profile-update-interval header
	SubTitle       string        // profile-title header
	DefaultHost    string        // address put into links when an inbound has none
	AdminUsername  string        // bootstrap sudo admin (created when no admins exist)
	AdminPassword  string
	CORSOrigins    []string
	JobsInterval   time.Duration
}

func Load() Config {
	return Config{
		Listen:         env("CLOUDRIX_LISTEN", ":8000"),
		DatabasePath:   env("CLOUDRIX_DB", "cloudrix.db"),
		JWTSecret:      env("CLOUDRIX_JWT_SECRET", ""),
		TokenTTL:       time.Duration(envInt("CLOUDRIX_TOKEN_TTL_HOURS", 24)) * time.Hour,
		SubPath:        "/" + strings.Trim(env("CLOUDRIX_SUB_PATH", "sub"), "/"),
		SubPublicURL:   strings.TrimRight(env("CLOUDRIX_SUB_URL", ""), "/"),
		SubUpdateHours: envInt("CLOUDRIX_SUB_UPDATE_HOURS", 12),
		SubTitle:       env("CLOUDRIX_SUB_TITLE", "Cloudrix"),
		DefaultHost:    env("CLOUDRIX_HOST", "127.0.0.1"),
		AdminUsername:  env("CLOUDRIX_ADMIN_USERNAME", ""),
		AdminPassword:  env("CLOUDRIX_ADMIN_PASSWORD", ""),
		CORSOrigins:    splitList(env("CLOUDRIX_CORS_ORIGINS", "")),
		JobsInterval:   time.Duration(envInt("CLOUDRIX_JOBS_INTERVAL_SECONDS", 30)) * time.Second,
	}
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return def
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
