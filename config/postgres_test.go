package config

import (
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestPostgresURL(t *testing.T) {
	values := validValues()
	values["POSTGRES_USER"] = "example@user"
	values["POSTGRES_PASSWORD"] = "password:/?#@ $"
	values["POSTGRES_HOST"] = "::1"
	dsn, err := postgresURL(values)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal("invalid generated URL")
	}
	password, _ := parsed.User.Password()
	if parsed.Host != "[::1]:5432" || parsed.User.Username() != values["POSTGRES_USER"] ||
		password != values["POSTGRES_PASSWORD"] ||
		parsed.Query().Get("sslmode") != "disable" {
		t.Fatal("Postgres URL did not preserve connection settings")
	}
	values["POSTGRES_SSLMODE"] = "verify-full"
	dsn, err = postgresURL(values)
	if err != nil || !strings.Contains(dsn, "sslmode=verify-full") {
		t.Fatal("SSL mode was not preserved")
	}
}

func TestPostgresURLValidation(t *testing.T) {
	for _, key := range []string{"POSTGRES_HOST", postgresPortKey, "POSTGRES_USER", "POSTGRES_PASSWORD", postgresDBKey} {
		t.Run(key, func(t *testing.T) {
			values := validValues()
			delete(values, key)
			if _, err := postgresURL(values); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("missing-key error=%v", err)
			}
		})
	}
	for _, key := range []string{postgresPortKey, "POSTGRES_SSLMODE"} {
		t.Run("invalid "+key, func(t *testing.T) {
			values := validValues()
			values[key] = "private-value"
			if _, err := postgresURL(values); err == nil || strings.Contains(err.Error(), "private-value") {
				t.Fatal("validation failed or disclosed an input value")
			}
		})
	}
}

func TestMigrationConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	for key, value := range validValues() {
		if strings.HasPrefix(key, "POSTGRES_") {
			t.Setenv(key, value)
		} else {
			t.Setenv(key, "")
		}
	}
	t.Setenv("POSTGRES_SSLMODE", "")
	t.Setenv("POSTGRES_URL", "")
	if _, err := LoadPostgresURL(); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMigrationURL(); err != nil {
		t.Fatal(err)
	}
	override := "postgres://override/db?sslmode=require"
	t.Setenv("POSTGRES_URL", override)
	t.Setenv("POSTGRES_PASSWORD", "")
	if got, err := LoadMigrationURL(); err != nil || got != override {
		t.Fatal("migration URL override did not take precedence")
	}
	if _, err := LoadPostgresURL(); err == nil {
		t.Fatal("application connection must not use the migration-only override")
	}
}

func TestMigrationDotenv(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ENV_FILE", dotenvPath)
	t.Setenv("POSTGRES_URL", "")
	if err := os.Unsetenv("POSTGRES_URL"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dotenvPath, []byte("POSTGRES_URL=postgres://example/db\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadMigrationURL(); err != nil || got != "postgres://example/db" {
		t.Fatal("dotenv migration override was not loaded")
	}
	if err := os.WriteFile(dotenvPath, []byte("POSTGRES_PASSWORD='private-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMigrationURL(); err == nil || strings.Contains(err.Error(), "private-value") {
		t.Fatal("malformed dotenv must fail without leaking values")
	}
	if _, err := LoadPostgresURL(); err == nil {
		t.Fatal("malformed dotenv must fail for application connection loading")
	}
}
