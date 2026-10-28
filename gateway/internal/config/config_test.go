package config_test

import (
	"slices"
	"testing"

	"github.com/caarlos0/env/v11"
	"github.com/vexner67/freenet/gateway/internal/config"
	"github.com/vexner67/freenet/pkg/envconfig"
)

func TestParse(t *testing.T) {
	t.Parallel()

	base := map[string]string{
		"HTTP_PORT":                 "8080",
		"LOG_LEVEL":                 "info",
		"LOG_FORMAT":                "text",
		"AUTH_SERVICE_ADDR":         "localhost:50051",
		"USER_SERVICE_ADDR":         "localhost:50052",
		"POST_SERVICE_ADDR":         "localhost:50053",
		"MEDIA_SERVICE_ADDR":        "localhost:50054",
		"NOTIFICATION_SERVICE_ADDR": "localhost:50055",
		"MESSAGE_SERVICE_ADDR":      "localhost:50056",
		"CLUB_SERVICE_ADDR":         "localhost:7057",
		"WORLD_SERVICE_ADDR":        "localhost:7058",
	}

	cfg, err := envconfig.Parse[config.Config](env.Options{Environment: base})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.TrustForwardedFor || cfg.RequestTimeout.String() != "10s" || len(cfg.CORSAllowedOrigins) != 0 || cfg.LinkPreviewTimeout.String() != "4s" {
		t.Fatalf("defaults: %+v", cfg)
	}

	withOrigins := map[string]string{"CORS_ALLOWED_ORIGINS": "http://localhost:5173,https://freenet.example"}
	for k, v := range base {
		withOrigins[k] = v
	}

	cfg, err = envconfig.Parse[config.Config](env.Options{Environment: withOrigins})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cfg.CORSAllowedOrigins, []string{"http://localhost:5173", "https://freenet.example"}) {
		t.Fatalf("origins = %v", cfg.CORSAllowedOrigins)
	}

	partial := map[string]string{"TLS_CA_PATH": "ca.pem"}
	for k, v := range base {
		partial[k] = v
	}

	if _, err = envconfig.Parse[config.Config](env.Options{Environment: partial}); err == nil {
		t.Fatal("partial mTLS must fail")
	}

	// The card of a link is read within the request that writes the post.
	for _, timeout := range []string{"-1s", "10s"} {
		slow := map[string]string{"LINK_PREVIEW_TIMEOUT": timeout}
		for k, v := range base {
			slow[k] = v
		}

		if _, err = envconfig.Parse[config.Config](env.Options{Environment: slow}); err == nil {
			t.Fatalf("LINK_PREVIEW_TIMEOUT=%s must fail", timeout)
		}
	}
}
