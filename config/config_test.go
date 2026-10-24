package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	postgresPortKey  = "POSTGRES_PORT"
	trueValue        = "true"
	sslKeyPathKey    = "SSL_KEY_PATH"
	dotenvPath       = ".env.dev"
	postgresDBKey    = "POSTGRES_DB"
	jwtSecretKey     = "JWT_SECRET"
	falseValue       = "false"
	developmentValue = "development"
	httpPortKey      = "HTTP_PORT"
	debugKey         = "DEBUG"
)

func validValues() map[string]string {
	return map[string]string{
		httpPortKey: "8081", "POSTGRES_HOST": "localhost", postgresPortKey: "5432",
		"POSTGRES_USER": "example", "POSTGRES_PASSWORD": "test-password", postgresDBKey: "example",
		"KAFKA_BROKER": "localhost:9092", "KAFKA_TOPIC": "dummy-events", jwtSecretKey: "test-secret",
		debugKey: trueValue,
	}
}

func TestRequiredValues(t *testing.T) {
	for key := range validValues() {
		if key == debugKey {
			continue
		}
		t.Run(key, func(t *testing.T) {
			values := validValues()
			delete(values, key)
			_, err := parse(values)
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("expected missing-key error for %s, got %v", key, err)
			}
		})
	}
}

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		name      string
		key       string
		value     string
		remove    bool
		wantError bool
	}{
		{"valid", debugKey, trueValue, false, false},
		{"debug skips TLS", debugKey, trueValue, false, false},
		{"default requires TLS", debugKey, "", true, true},
		{"false requires TLS", debugKey, falseValue, false, true},
		{"invalid debug", debugKey, "not-a-boolean", false, true},
		{"empty debug", debugKey, "", false, true},
		{"zero HTTP", httpPortKey, "0", false, true},
		{"negative HTTP", httpPortKey, "-1", false, true},
		{"large HTTP", httpPortKey, "65536", false, true},
		{"noninteger HTTP", httpPortKey, "8080x", false, true},
		{"maximum HTTP", httpPortKey, "65535", false, false},
		{"minimum HTTP", httpPortKey, "1", false, false},
		{"zero Postgres", postgresPortKey, "0", false, true},
		{"large Postgres", postgresPortKey, "65536", false, true},
		{"noninteger Postgres", postgresPortKey, "secret-value", false, true},
		{"blank secret", jwtSecretKey, " ", false, true},
		{"blank broker", "KAFKA_BROKER", " ", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := validValues()
			if tc.remove {
				delete(values, tc.key)
			} else {
				values[tc.key] = tc.value
			}
			cfg, err := parse(values)
			if (err != nil) != tc.wantError {
				t.Fatalf("error=%v", err)
			}
			if err != nil && strings.Contains(err.Error(), "secret-value") {
				t.Fatal("error leaked an environment value")
			}
			if err == nil && cfg.KafkaGroupID != "dummy-service" {
				t.Fatal("missing default group")
			}
		})
	}
}

func TestTLSAndGroup(t *testing.T) {
	values := validValues()
	delete(values, debugKey)
	values["SSL_CERT_PATH"], values[sslKeyPathKey] = "example.crt", "example.key"
	values["KAFKA_GROUP_ID"] = "custom-group"
	cfg, err := parse(values)
	if err != nil || cfg.Debug || cfg.KafkaGroupID != "custom-group" || cfg.SSLCertPath != "example.crt" ||
		cfg.SSLKeyPath != "example.key" {
		t.Fatalf("invalid TLS/group configuration: %v", err)
	}
	delete(values, sslKeyPathKey)
	if _, err := parse(values); err == nil || !strings.Contains(err.Error(), sslKeyPathKey) {
		t.Fatalf("expected missing TLS key, got %v", err)
	}
}

func setEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("ENV_FILE", "")
	for key, value := range validValues() {
		t.Setenv(key, value)
	}
	t.Setenv("KAFKA_GROUP_ID", "")
	t.Setenv("SSL_CERT_PATH", "")
	t.Setenv(sslKeyPathKey, "")
}

func TestLoad(t *testing.T) {
	for _, tc := range []struct {
		name      string
		file      string
		wantError bool
	}{
		{"no dotenv", "", false},
		{"existing dotenv", "HTTP_PORT=9090\nJWT_SECRET=file-secret\n", false},
		{"malformed dotenv", "JWT_SECRET='file-secret\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			setEnvironment(t)
			if tc.file != "" {
				if err := os.WriteFile(dotenvPath, []byte(tc.file), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := Load()
			if (err != nil) != tc.wantError {
				t.Fatalf("error=%v", err)
			}
			if err != nil && strings.Contains(err.Error(), "file-secret") {
				t.Fatal("dotenv error leaked a value")
			}
			if err == nil &&
				(cfg.HTTPPort != 8081 || cfg.JWTSecret != "test-secret" || cfg.KafkaTopic != "dummy-events") {
				t.Fatal("environment values were not preserved")
			}
			if os.Getenv(httpPortKey) != "8081" {
				t.Fatal("Load mutated the environment")
			}
		})
	}
}

func TestDotenvFallback(t *testing.T) {
	t.Chdir(t.TempDir())
	setEnvironment(t)
	if err := os.Unsetenv(httpPortKey); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dotenvPath, []byte("HTTP_PORT=9090\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil || cfg.HTTPPort != 9090 {
		t.Fatalf("dotenv fallback failed: %v", err)
	}
	if _, exists := os.LookupEnv(httpPortKey); exists {
		t.Fatal("Load mutated the environment")
	}
}

func TestUnreadableDotenv(t *testing.T) {
	t.Chdir(t.TempDir())
	setEnvironment(t)
	if err := os.Mkdir(filepath.Join(".", dotenvPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("expected an error for a directory named " + dotenvPath)
	}
}

func TestEnvironmentSelection(t *testing.T) {
	for _, tc := range []struct {
		name  string
		debug string
		file  string
		want  string
	}{
		{"development default", "", "", developmentValue},
		{"production selector", falseValue, "", "production"},
		{"explicit development", "true", "", developmentValue},
		{"explicit production file", "", ".env.prod", "production"},
		{"file override wins selector", falseValue, dotenvPath, developmentValue},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			setEnvironment(t)
			t.Setenv("ENV_FILE", tc.file)
			t.Setenv("POSTGRES_URL", "")
			if tc.debug == "" {
				if err := os.Unsetenv(debugKey); err != nil {
					t.Fatal(err)
				}
			} else {
				t.Setenv(debugKey, tc.debug)
			}
			for _, key := range []string{postgresDBKey, jwtSecretKey} {
				if err := os.Unsetenv(key); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("SSL_CERT_PATH", "certs/server.crt")
			t.Setenv(sslKeyPathKey, "certs/server.key")
			for path, data := range map[string]string{
				dotenvPath:  "POSTGRES_DB=development\nJWT_SECRET=development-secret\nDEBUG=true\n",
				".env.prod": "POSTGRES_DB=production\nJWT_SECRET=production-secret\nDEBUG=false\n",
				".env":      "POSTGRES_DB=legacy\nJWT_SECRET=legacy-secret\n",
			} {
				if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := Load()
			if err != nil || cfg.PostgresDB != tc.want || cfg.JWTSecret != tc.want+"-secret" {
				t.Fatalf("selected wrong environment: %v", err)
			}
			dsn, err := LoadMigrationURL()
			if err != nil || !strings.Contains(dsn, "/"+tc.want+"?") {
				t.Fatalf("migration selected wrong environment: %v", err)
			}
		})
	}
}
