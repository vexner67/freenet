package config_test

import (
	"testing"

	"github.com/caarlos0/env/v11"
	"github.com/vexner67/freenet/pkg/envconfig"
	"github.com/vexner67/freenet/post/internal/config"
)

func vars(extra map[string]string) map[string]string {
	m := map[string]string{
		"GRPC_PORT":           "50053",
		"LOG_LEVEL":           "info",
		"LOG_FORMAT":          "text",
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
		{"full mtls", map[string]string{"TLS_CA_PATH": "ca.pem", "TLS_CERT_PATH": "c.pem", "TLS_KEY_PATH": "k.pem"}, true},
		{"partial mtls", map[string]string{"TLS_KEY_PATH": "key.pem"}, false},
		{"empty issuer", map[string]string{"JWT_ISSUER": ""}, false},
		{"postponed posts every minute", map[string]string{"POSTPONED_INTERVAL": "1m"}, true},
		{"postponed posts never", map[string]string{"POSTPONED_INTERVAL": "0s"}, false},
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

func TestParse_RateLimits(t *testing.T) {
	t.Parallel()

	cfg, err := envconfig.Parse[config.Config](env.Options{Environment: vars(nil)})
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.PostLimits.String(); got != "5/1m0s,100/24h0m0s" {
		t.Fatalf("default post limits = %s", got)
	}

	cfg, err = envconfig.Parse[config.Config](env.Options{Environment: vars(map[string]string{"RATE_LIMIT_POSTS": "1/1s"})})
	if err != nil || cfg.PostLimits.String() != "1/1s" {
		t.Fatalf("custom limits = %s, %v", cfg.PostLimits, err)
	}

	if _, err = envconfig.Parse[config.Config](env.Options{Environment: vars(map[string]string{"RATE_LIMIT_LIKES": "often"})}); err == nil {
		t.Fatal("bad limits must fail the start")
	}
}
