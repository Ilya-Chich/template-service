package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPPort         int
	Debug            bool
	PostgresHost     string
	PostgresPort     int
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
	KafkaBroker      string
	KafkaTopic       string
	KafkaGroupID     string
	JWTSecret        string
	SSLCertPath      string
	SSLKeyPath       string
}

func Load() (Config, error) {
	values, err := loadEnvironment()
	if err != nil {
		return Config{}, err
	}
	return parse(values)
}

func loadEnvironment() (map[string]string, error) {
	path := os.Getenv("ENV_FILE")
	if path == "" {
		path = ".env.dev"
		if debug, err := strconv.ParseBool(os.Getenv("DEBUG")); err == nil && !debug {
			path = ".env.prod"
		}
	}
	values, err := godotenv.Read(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, errors.New("cannot read or parse environment file")
	}
	if values == nil {
		values = make(map[string]string)
	}
	for _, entry := range os.Environ() {
		key, value, found := strings.Cut(entry, "=")
		if found {
			values[key] = value
		}
	}
	return values, nil
}

func parse(values map[string]string) (Config, error) {
	for _, key := range []string{"HTTP_PORT", "POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB", "KAFKA_BROKER", "KAFKA_TOPIC", "JWT_SECRET"} {
		if strings.TrimSpace(values[key]) == "" {
			return Config{}, fmt.Errorf("missing required environment variable: %s", key)
		}
	}
	httpPort, err := parsePort(values["HTTP_PORT"], "HTTP_PORT")
	if err != nil {
		return Config{}, err
	}
	postgresPort, err := parsePort(values["POSTGRES_PORT"], "POSTGRES_PORT")
	if err != nil {
		return Config{}, err
	}
	debug := false
	if value, present := values["DEBUG"]; present {
		debug, err = strconv.ParseBool(value)
		if err != nil {
			return Config{}, errors.New("DEBUG must be a boolean")
		}
	}
	if !debug {
		for _, key := range []string{"SSL_CERT_PATH", "SSL_KEY_PATH"} {
			if strings.TrimSpace(values[key]) == "" {
				return Config{}, fmt.Errorf("missing required environment variable: %s", key)
			}
		}
	}
	groupID := values["KAFKA_GROUP_ID"]
	if strings.TrimSpace(groupID) == "" {
		groupID = "dummy-service"
	}
	return Config{
		HTTPPort: httpPort, Debug: debug,
		PostgresHost: values["POSTGRES_HOST"], PostgresPort: postgresPort,
		PostgresUser: values["POSTGRES_USER"], PostgresPassword: values["POSTGRES_PASSWORD"], PostgresDB: values["POSTGRES_DB"],
		KafkaBroker: values["KAFKA_BROKER"], KafkaTopic: values["KAFKA_TOPIC"], KafkaGroupID: groupID,
		JWTSecret: values["JWT_SECRET"], SSLCertPath: values["SSL_CERT_PATH"], SSLKeyPath: values["SSL_KEY_PATH"],
	}, nil
}

func parsePort(value, key string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("%s must be a port between 1 and 65535", key)
	}
	return port, nil
}
