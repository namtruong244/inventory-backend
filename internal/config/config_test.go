package config_test

import (
	"strings"
	"testing"

	"inventory_backend/internal/config"
)

func TestConfigValidate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     config.Config
		wantErr bool
	}{
		{"dev allows default secret", config.Config{AppEnv: "development", JWTSecret: "default_secret_inventory_system_key"}, false},
		{"prod rejects default secret", config.Config{AppEnv: "production", JWTSecret: "default_secret_inventory_system_key"}, true},
		{"prod rejects documented placeholder", config.Config{AppEnv: "production", JWTSecret: "super_secret_jwt_key_change_in_production_1234567890"}, true},
		{"prod rejects short secret", config.Config{AppEnv: "production", JWTSecret: "short"}, true},
		{"prod accepts strong secret", config.Config{AppEnv: "production", JWTSecret: strings.Repeat("k", 40)}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.Validate(); (err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v, got %v", tc.wantErr, err)
			}
		})
	}
}
