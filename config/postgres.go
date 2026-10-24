package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func LoadPostgresURL() (string, error) {
	values, err := loadEnvironment()
	if err != nil {
		return "", err
	}
	return postgresURL(values)
}
func LoadMigrationURL() (string, error) {
	values, err := loadEnvironment()
	if err != nil {
		return "", err
	}
	if value := values["POSTGRES_URL"]; strings.TrimSpace(value) != "" {
		return value, nil
	}
	return postgresURL(values)
}
func postgresURL(values map[string]string) (string, error) {
	for _, key := range []string{"POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB"} {
		if strings.TrimSpace(values[key]) == "" {
			return "", fmt.Errorf("missing required environment variable: %s", key)
		}
	}
	port, err := parsePort(values["POSTGRES_PORT"], "POSTGRES_PORT")
	if err != nil {
		return "", err
	}
	sslMode := values["POSTGRES_SSLMODE"]
	if sslMode == "" {
		sslMode = "disable"
	}
	switch sslMode {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
	default:
		return "", fmt.Errorf("POSTGRES_SSLMODE is invalid")
	}
	dsn := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(values["POSTGRES_USER"], values["POSTGRES_PASSWORD"]),
		Host:   net.JoinHostPort(values["POSTGRES_HOST"], strconv.Itoa(port)),
		Path:   "/" + values["POSTGRES_DB"],
	}
	query := dsn.Query()
	query.Set("sslmode", sslMode)
	dsn.RawQuery = query.Encode()
	return dsn.String(), nil
}
