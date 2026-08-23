package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestFromEnv(t *testing.T) {
	t.Setenv(EnvPostgresDSN, "postgres://muni:secret@db/muni")
	t.Setenv(EnvBootstrapAdmin, "Admin@Example.com")
	t.Setenv(EnvBootstrapAdminPassword, "correct-horse-battery-staple")
	t.Setenv(EnvEncryptionKey, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv returned an error: %v", err)
	}
	if cfg.BootstrapAdmin != "admin@example.com" {
		t.Fatalf("identity was not normalized: %q", cfg.BootstrapAdmin)
	}
	if len(cfg.EncryptionKey) != 32 {
		t.Fatalf("unexpected encryption key length: %d", len(cfg.EncryptionKey))
	}
}

func TestFromEnvRejectsWeakConfiguration(t *testing.T) {
	t.Setenv(EnvPostgresDSN, "postgres://db/muni")
	t.Setenv(EnvBootstrapAdmin, "admin")
	t.Setenv(EnvBootstrapAdminPassword, "short")
	t.Setenv(EnvEncryptionKey, "not-base64")
	_, err := FromEnv()
	if err == nil || !strings.Contains(err.Error(), "12") {
		t.Fatalf("expected password validation error, got %v", err)
	}
}
