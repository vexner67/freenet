package config_test

import (
	"testing"

	"github.com/caarlos0/env/v11"
	"github.com/vexner67/freenet/media/internal/config"
	"github.com/vexner67/freenet/pkg/envconfig"
)

func vars(extra map[string]string) map[string]string {
	m := map[string]string{
		"GRPC_PORT":           "50054",
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
		{"inmem by default, nothing to set up", nil, true},
		{"persistent with a directory", map[string]string{"STORAGE": config.StoragePersistent, "MEDIA_DIR": "/tmp/media"}, true},
		{"persistent without a directory", map[string]string{"STORAGE": config.StoragePersistent}, false},
		{"unknown storage", map[string]string{"STORAGE": "files"}, false},
		{"full mtls", map[string]string{"TLS_CA_PATH": "ca.pem", "TLS_CERT_PATH": "c.pem", "TLS_KEY_PATH": "k.pem"}, true},
		{"partial mtls", map[string]string{"TLS_KEY_PATH": "key.pem"}, false},
		{"empty issuer", map[string]string{"JWT_ISSUER": ""}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := envconfig.Parse[config.Config](env.Options{Environment: vars(tt.extra)})
			if (err == nil) != tt.ok {
				t.Fatalf("err = %v, want ok = %v", err, tt.ok)
			}
			if tt.ok && cfg.Storage == "" {
				t.Fatal("storage must default to inmem")
			}
		})
	}
}
