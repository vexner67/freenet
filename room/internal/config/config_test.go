package config_test

import (
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/room/internal/config"
	"testing"
)

func TestRoomConfig(t *testing.T) {
	t.Setenv("JWT_PUBLIC_KEY_PATH", "test.pem")
	t.Setenv("JWT_ISSUER", "test")
	t.Setenv("LIVEKIT_API_KEY", "devkey")
	t.Setenv("LIVEKIT_API_SECRET", "test-secret")
	c, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.MaxMembers != 5 || c.MaxLive != 20 || c.GRPCPort != 7059 {
		t.Fatal(c.MaxMembers, c.MaxLive, c.GRPCPort)
	}
	for _, change := range []func(*config.Config){func(c *config.Config) { c.MaxMembers = 0 }, func(c *config.Config) { c.GRPCPort = 50059 }, func(c *config.Config) { c.LiveKitURL = "http://user:password@localhost" }, func(c *config.Config) { c.LiveKitPublicURL = "https://example.com" }, func(c *config.Config) { c.Storage = config.StoragePersistent }, func(c *config.Config) { c.StateTTL = c.EmptyTTL }} {
		next := c
		change(&next)
		if err := next.Validate(); err == nil {
			t.Fatal("invalid room config accepted")
		}
	}
	c.Storage = config.StoragePersistent
	c.DatabaseURL = "postgres://localhost/test"
	c.RedisAddr = "localhost:6379"
	if err := c.Validate(); err == nil {
		t.Fatal("persistent rooms without mTLS")
	}
	c.TLS = mtls.Config{CAPath: "ca", CertPath: "room", KeyPath: "key"}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}
