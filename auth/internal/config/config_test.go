package config_test

import (
	"testing"

	"github.com/caarlos0/env/v11"
	"github.com/vexner67/freenet/auth/internal/config"
	"github.com/vexner67/freenet/pkg/envconfig"
)

func base() map[string]string {
	return map[string]string{
		"GRPC_PORT":            "50051",
		"LOG_LEVEL":            "info",
		"LOG_FORMAT":           "text",
		"USER_SERVICE_ADDR":    "localhost:50052",
		"HASH_SECRET":          "secret",
		"JWT_PRIVATE_KEY_PATH": "private.pem",
		"JWT_PUBLIC_KEY_PATH":  "public.pem",
		"ISSUER":               "freenet-auth",
	}
}

func with(extra map[string]string) map[string]string {
	m := base()
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestParse(t *testing.T) {
	t.Parallel()

	t.Run("defaults are development friendly", func(t *testing.T) {
		t.Parallel()

		cfg, err := envconfig.Parse[config.Config](env.Options{Environment: base()})
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if cfg.Storage != config.StorageInmem || cfg.Mailer != config.MailerConsole {
			t.Fatalf("storage=%q mailer=%q", cfg.Storage, cfg.Mailer)
		}
		if cfg.Code.MaxAttempts != 5 || cfg.Code.RequestsPerIP != 10 || cfg.Code.TTL.String() != "5m0s" {
			t.Fatalf("code defaults: %+v", cfg.Code)
		}
	})

	t.Run("empty optional values fall back to defaults", func(t *testing.T) {
		t.Parallel()

		// As after copying .env.example without filling the optional part.
		cfg, err := envconfig.Parse[config.Config](env.Options{Environment: with(map[string]string{
			"STORAGE":           "",
			"MAILER":            "",
			"CODE_TTL":          "",
			"CODE_MAX_ATTEMPTS": "",
			"ACCESS_TOKEN_TTL":  "",
		})})
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if cfg.Storage != config.StorageInmem || cfg.Code.MaxAttempts != 5 || cfg.AccessTokenTTL.String() != "15m0s" {
			t.Fatalf("defaults not applied: %+v", cfg)
		}
	})

	t.Run("persistent with everything it needs", func(t *testing.T) {
		t.Parallel()

		_, err := envconfig.Parse[config.Config](env.Options{Environment: with(map[string]string{
			"STORAGE":      config.StoragePersistent,
			"DATABASE_URL": "postgres://localhost/auth",
			"REDIS_ADDR":   "localhost:6379",
			"MAILER":       config.MailerSMTP,
			"SMTP_HOST":    "localhost",
			"SMTP_PORT":    "1025",
			"SMTP_FROM":    "noreply@freenet.local",
		})})
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
	})

	t.Run("codes to the owner's telegram chat", func(t *testing.T) {
		t.Parallel()

		cfg, err := envconfig.Parse[config.Config](env.Options{Environment: with(map[string]string{
			"MAILER":             config.MailerTelegram,
			"TELEGRAM_BOT_TOKEN": "1:token",
			"TELEGRAM_CHAT_ID":   "-100",
		})})
		if err != nil || cfg.Telegram.API != "https://api.telegram.org" {
			t.Fatalf("parse: %+v, %v", cfg.Telegram.API, err)
		}
	})

	invalid := []struct {
		name  string
		extra map[string]string
	}{
		{"unknown storage", map[string]string{"STORAGE": "mongo"}},
		{"persistent without redis", map[string]string{"STORAGE": config.StoragePersistent, "DATABASE_URL": "postgres://x"}},
		{"persistent without database", map[string]string{"STORAGE": config.StoragePersistent, "REDIS_ADDR": "x:6379"}},
		{"unknown mailer", map[string]string{"MAILER": "pigeon"}},
		{"smtp without host", map[string]string{"MAILER": config.MailerSMTP, "SMTP_PORT": "25", "SMTP_FROM": "a@b.c"}},
		{"telegram without a chat", map[string]string{"MAILER": config.MailerTelegram, "TELEGRAM_BOT_TOKEN": "1:token"}},
		{"telegram without a bot", map[string]string{"MAILER": config.MailerTelegram, "TELEGRAM_CHAT_ID": "-100"}},
		{"partial mtls", map[string]string{"TLS_CERT_PATH": "cert.pem"}},
		{"zero attempts", map[string]string{"CODE_MAX_ATTEMPTS": "0"}},
		{"empty required", map[string]string{"ISSUER": ""}},
	}

	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := envconfig.Parse[config.Config](env.Options{Environment: with(tt.extra)}); err == nil {
				t.Fatal("expected an error")
			}
		})
	}

	t.Run("missing required", func(t *testing.T) {
		t.Parallel()

		vars := base()
		delete(vars, "HASH_SECRET")

		if _, err := envconfig.Parse[config.Config](env.Options{Environment: vars}); err == nil {
			t.Fatal("expected an error")
		}
	})
}
