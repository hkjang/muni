package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	EnvPostgresDSN            = "POSTGRES_DSN"
	EnvBootstrapAdmin         = "BOOTSTRAP_ADMIN"
	EnvBootstrapAdminPassword = "BOOTSTRAP_ADMIN_PASSWORD"
	EnvEncryptionKey          = "ENCRYPTION_KEY"
)

type Config struct {
	PostgresDSN            string
	BootstrapAdmin         string
	BootstrapAdminPassword string
	EncryptionKey          []byte
}

func FromEnv() (Config, error) {
	cfg := Config{
		PostgresDSN:            strings.TrimSpace(os.Getenv(EnvPostgresDSN)),
		BootstrapAdmin:         strings.ToLower(strings.TrimSpace(os.Getenv(EnvBootstrapAdmin))),
		BootstrapAdminPassword: os.Getenv(EnvBootstrapAdminPassword),
	}
	var missing []string
	if cfg.PostgresDSN == "" {
		missing = append(missing, EnvPostgresDSN)
	}
	if cfg.BootstrapAdmin == "" {
		missing = append(missing, EnvBootstrapAdmin)
	}
	if cfg.BootstrapAdminPassword == "" {
		missing = append(missing, EnvBootstrapAdminPassword)
	}
	rawKey := strings.TrimSpace(os.Getenv(EnvEncryptionKey))
	if rawKey == "" {
		missing = append(missing, EnvEncryptionKey)
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("required environment variables are missing: %s", strings.Join(missing, ", "))
	}
	if !strings.Contains(cfg.BootstrapAdmin, "@") && len(cfg.BootstrapAdmin) < 3 {
		return Config{}, errors.New("BOOTSTRAP_ADMIN must be an email address or a username with at least 3 characters")
	}
	if len(cfg.BootstrapAdminPassword) < 12 {
		return Config{}, errors.New("BOOTSTRAP_ADMIN_PASSWORD must contain at least 12 characters")
	}
	key, err := base64.StdEncoding.DecodeString(rawKey)
	if err != nil || len(key) != 32 {
		return Config{}, errors.New("ENCRYPTION_KEY must be a base64-encoded 32-byte key (generate with: openssl rand -base64 32)")
	}
	cfg.EncryptionKey = key
	return cfg, nil
}
