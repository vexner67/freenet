package envconfig_test

import (
	"errors"
	"testing"

	"github.com/caarlos0/env/v11"
	"github.com/vexner67/freenet/pkg/envconfig"
)

type config struct {
	Port int    `env:"PORT,required,notEmpty"`
	Mode string `env:"MODE" envDefault:"dev"`
}

func (c config) Validate() error {
	if c.Mode != "dev" && c.Mode != "prod" {
		return errors.New("bad mode")
	}
	return nil
}

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		vars map[string]string
		ok   bool
	}{
		{"defaults", map[string]string{"PORT": "80"}, true},
		{"missing required", map[string]string{}, false},
		{"empty required", map[string]string{"PORT": ""}, false},
		{"validation fails", map[string]string{"PORT": "80", "MODE": "test"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := envconfig.Parse[config](env.Options{Environment: tt.vars})
			if (err == nil) != tt.ok {
				t.Fatalf("err = %v, want ok = %v", err, tt.ok)
			}
			if tt.ok && cfg.Mode != "dev" {
				t.Fatalf("default not applied: %+v", cfg)
			}
		})
	}
}
