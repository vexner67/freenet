package config_test

import (
	"testing"

	"github.com/caarlos0/env/v11"
	"github.com/vexner67/freenet/federation/internal/config"
	"github.com/vexner67/freenet/pkg/envconfig"
)

func vars(extra map[string]string) map[string]string {
	m := map[string]string{
		"GRPC_PORT":           "7058",
		"LOG_LEVEL":           "info",
		"LOG_FORMAT":          "text",
		"USER_SERVICE_ADDR":   "localhost:7052",
		"POST_SERVICE_ADDR":   "localhost:7053",
		"JWT_PUBLIC_KEY_PATH": "public.pem",
		"JWT_ISSUER":          "freenet-auth",
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		extra map[string]string
		ok    bool
	}{
		{"inmem by default, no database needed", nil, true},
		{"persistent with a database", map[string]string{"STORAGE": config.StoragePersistent, "DATABASE_URL": "postgres://x"}, true},
		{"persistent without a database", map[string]string{"STORAGE": config.StoragePersistent}, false},
		{"unknown storage", map[string]string{"STORAGE": "files"}, false},
		{"the domain of the site", map[string]string{"PUBLIC_URL": "https://freenet19.fun"}, true},
		{"no domain", map[string]string{"PUBLIC_URL": "https://:80"}, false},
		{"a path", map[string]string{"PUBLIC_URL": "https://freenet19.fun/site"}, false},
		{"no user service", map[string]string{"USER_SERVICE_ADDR": ""}, false},
		{"no post service", map[string]string{"POST_SERVICE_ADDR": ""}, false},
		{"no remote timeout", map[string]string{"REMOTE_TIMEOUT": "0"}, false},
		{"no delivery age", map[string]string{"DELIVERY_MAX_AGE": "-1h"}, false},
		{"partial mtls", map[string]string{"TLS_KEY_PATH": "key.pem"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := envconfig.Parse[config.Config](env.Options{Environment: vars(tt.extra)})
			if (err == nil) != tt.ok {
				t.Fatalf("err = %v, want ok = %v", err, tt.ok)
			}
			if tt.ok && !cfg.MigrateOnStart {
				t.Fatal("migrations must run on start by default")
			}
		})
	}
}
