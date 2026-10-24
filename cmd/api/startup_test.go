package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"http-api-template/config"
	logadapter "http-api-template/internal/adapter/logger"
	"http-api-template/internal/adapter/uuid"
)

func TestExpectedStartupCancellation(t *testing.T) {
	t.Chdir(t.TempDir())
	const fixtureValue = "test"
	for key, value := range map[string]string{
		"HTTP_PORT": "8081", "DEBUG": "true",
		"POSTGRES_HOST": "localhost", "POSTGRES_PORT": "1",
		"POSTGRES_USER": fixtureValue, "POSTGRES_PASSWORD": fixtureValue, "POSTGRES_DB": fixtureValue,
		"POSTGRES_SSLMODE": "disable", "KAFKA_BROKER": "localhost:9092",
		"KAFKA_TOPIC": fixtureValue, "JWT_SECRET": fixtureValue,
	} {
		t.Setenv(key, value)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := run(ctx); err != nil {
		t.Fatalf("expected startup cancellation to be normal: %v", err)
	}
	// A signal must not suppress an independent configuration failure.
	t.Setenv("HTTP_PORT", "invalid")
	if err := run(ctx); err == nil {
		t.Fatal("configuration error lost during cancellation")
	}
}

func TestStartupCancellationClassification(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	closeErr := errors.New("database close failed")
	for _, tc := range []struct {
		name string
		ctx  context.Context
		err  error
		want bool
	}{
		{"signal", ctx, context.Canceled, true},
		{"wrapped signal", ctx, fmt.Errorf("ping: %w", context.Canceled), true},
		{"closed successfully", ctx, errors.Join(context.Canceled, nil), true},
		{"multiple cancellations", ctx, errors.Join(context.Canceled, fmt.Errorf("ping: %w", context.Canceled)), true},
		{"cleanup failed", ctx, errors.Join(context.Canceled, closeErr), false},
		{"wrapped cleanup failed", ctx, fmt.Errorf("open: %w", errors.Join(context.Canceled, closeErr)), false},
		{"unrelated failure", ctx, closeErr, false},
		{"startup timeout", ctx, context.DeadlineExceeded, false},
		{"no signal", context.Background(), context.Canceled, false},
		{"no error", ctx, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := expectedCancellation(tc.ctx, tc.err); got != tc.want {
				t.Fatalf("expectedCancellation(%v)=%v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestStartupFailureClosesDatabaseAndReportsCleanupError(t *testing.T) {
	for _, stage := range []string{"writer", "reader", "TLS listener"} {
		t.Run(stage, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
			if err != nil {
				t.Fatal(err)
			}
			closeErr := errors.New("database close failed")
			mock.ExpectClose().WillReturnError(closeErr)
			cfg := config.Config{KafkaBroker: "localhost:9092", KafkaTopic: "dummy-events", KafkaGroupID: "test", HTTPPort: 0,
				SSLCertPath: "missing-test-certificate", SSLKeyPath: "missing-test-key"}
			switch stage {
			case "writer":
				cfg.KafkaTopic = ""
			case "reader":
				cfg.KafkaGroupID = ""
			}
			ids := uuid.Generator{}
			err = serve(context.Background(), db, cfg, logadapter.New(io.Discard, ids), ids, nil, nil)
			if !errors.Is(err, closeErr) {
				t.Fatalf("cleanup error lost: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
