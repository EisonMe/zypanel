// Package config loads ZyPanel settings from the environment.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config is the fully resolved runtime configuration.
type Config struct {
	Addr string

	SecretKey         string
	AdminUser         string
	AdminPassword     string
	AdminPasswordHash string
	CookieSecure      bool

	DataDir       string
	AuditLogPath  string
	FavoritesPath string

	SessionTTL time.Duration

	DockerEnabled      bool
	SystemdEnabled     bool
	CloudflaredEnabled bool
	OpenClawEnabled    bool
	ReadOnly           bool

	DockerBin         string
	SystemctlBin      string
	CloudflaredBin    string
	CloudflaredConfig string
	OpenClawBin       string

	CloudflaredSuffixAllowlist []string
}

func envString(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

// Load reads configuration from the environment and validates it.
func Load() (*Config, error) {
	dataDir := envString("ZYPANEL_DATA_DIR", "/data")

	cfg := &Config{
		Addr: envString("ZYPANEL_ADDR", ":8090"),

		SecretKey:         envString("ZYPANEL_SECRET_KEY", ""),
		AdminUser:         envString("ZYPANEL_ADMIN_USER", "admin"),
		AdminPassword:     envString("ZYPANEL_ADMIN_PASSWORD", ""),
		AdminPasswordHash: envString("ZYPANEL_ADMIN_PASSWORD_HASH", ""),
		CookieSecure:      envBool("ZYPANEL_COOKIE_SECURE", true),

		DataDir:       dataDir,
		AuditLogPath:  envString("ZYPANEL_AUDIT_LOG", filepath.Join(dataDir, "audit.jsonl")),
		FavoritesPath: envString("ZYPANEL_FAVORITES", filepath.Join(dataDir, "favorites.json")),

		SessionTTL: 12 * time.Hour,

		DockerEnabled:      envBool("ZYPANEL_DOCKER", true),
		SystemdEnabled:     envBool("ZYPANEL_SYSTEMD", false),
		CloudflaredEnabled: envBool("ZYPANEL_CLOUDFLARED", false),
		OpenClawEnabled:    envBool("ZYPANEL_OPENCLAW", false),
		ReadOnly:           envBool("ZYPANEL_READ_ONLY", false),

		DockerBin:         envString("ZYPANEL_DOCKER_BIN", "docker"),
		SystemctlBin:      envString("ZYPANEL_SYSTEMCTL_BIN", "systemctl"),
		CloudflaredBin:    envString("ZYPANEL_CLOUDFLARED_BIN", "cloudflared"),
		CloudflaredConfig: envString("ZYPANEL_CLOUDFLARED_CONFIG", "/etc/cloudflared/config.yml"),
		OpenClawBin:       envString("ZYPANEL_OPENCLAW_BIN", "openclaw"),
	}

	if raw := envString("ZYPANEL_CLOUDFLARED_SUFFIX_ALLOWLIST", ""); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				cfg.CloudflaredSuffixAllowlist = append(cfg.CloudflaredSuffixAllowlist, strings.ToLower(part))
			}
		}
	}

	if cfg.SecretKey == "" {
		return nil, fmt.Errorf("ZYPANEL_SECRET_KEY is required")
	}
	if len(cfg.SecretKey) < 16 {
		return nil, fmt.Errorf("ZYPANEL_SECRET_KEY must be at least 16 characters")
	}
	if cfg.AdminPassword == "" && cfg.AdminPasswordHash == "" {
		return nil, fmt.Errorf("set ZYPANEL_ADMIN_PASSWORD or ZYPANEL_ADMIN_PASSWORD_HASH")
	}
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.AuditLogPath), 0o750); err != nil {
		return nil, fmt.Errorf("create audit dir: %w", err)
	}
	return cfg, nil
}

// AuditPath returns the configured audit log file path.
func (c *Config) AuditPath() string { return c.AuditLogPath }

// ParseBool is a helper used by templates.
func ParseBool(s string) bool {
	b, err := strconv.ParseBool(s)
	return err == nil && b
}
