package main

import (
	"context"
	"errors"
	"http-api-template/config"
	authadapter "http-api-template/internal/adapter/auth"
	httpadapter "http-api-template/internal/adapter/http"
	kafkaadapter "http-api-template/internal/adapter/kafka"
	logadapter "http-api-template/internal/adapter/logger"
	"http-api-template/internal/adapter/postgres"
	"http-api-template/internal/adapter/uuid"
	"http-api-template/internal/core/port"
	"http-api-template/internal/core/service"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx)
	stop()
	if err != nil {
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	ids := uuid.Generator{}
	log := logadapter.New(os.Stdout, ids)
	cfg, err := config.Load()
	if err != nil {
		log.Error(context.Background(), "Invalid service configuration")
		return err
	}
	dsn, err := config.LoadPostgresURL()
	if err != nil {
		log.Error(context.Background(), "Invalid Postgres configuration")
		return err
	}
	tokens, err := authadapter.NewJWT(cfg.JWTSecret)
	if err != nil {
		log.Error(context.Background(), "Invalid JWT configuration")
		return err
	}
	startup, cancel := context.WithTimeout(ctx, 10*time.Second)
	db, err := postgres.Open(startup, dsn)
	cancel()
	if err != nil {
		if expectedCancellation(ctx, err) {
			return nil
		}
		log.Error(context.Background(), "Cannot open database")
		return err
	}
	if err := serve(ctx, db, cfg, log, ids, service.NewAuth(tokens), tokens); err != nil {
		log.Error(context.Background(), "API stopped with an error")
		return err
	}
	return nil
}
func serve(ctx context.Context, db *gorm.DB, cfg config.Config, log port.Logger, ids port.IDGenerator, auth port.AuthService, verifier port.TokenVerifier) (err error) {
	defer func() {
		err = errors.Join(err, bounded("database close", shutdownTimeout, func(context.Context) error { return postgres.Close(db) }))
	}()
	writer, err := kafkaadapter.NewWriter([]string{cfg.KafkaBroker}, cfg.KafkaTopic)
	if err != nil {
		return err
	}
	publisher := kafkaadapter.NewPublisher(writer, log, ids)
	defer func() {
		err = errors.Join(err, bounded("writer close", shutdownTimeout, func(context.Context) error { return publisher.Close() }))
	}()
	processor := service.NewDummy(postgres.NewDummy(db), publisher, ids)
	gin.SetMode(gin.ReleaseMode)
	router, err := httpadapter.NewRouter(processor, auth, verifier, log, ids)
	if err != nil {
		return err
	}
	reader, err := kafkaadapter.NewReader([]string{cfg.KafkaBroker}, cfg.KafkaTopic, cfg.KafkaGroupID)
	if err != nil {
		return err
	}
	consumer := kafkaadapter.NewConsumer(reader, processor, log, ids)
	server := newHTTPServer(":"+strconv.Itoa(cfg.HTTPPort), router)
	return runKafkaAndHTTP(ctx, consumer.Run, func() error {
		if cfg.Debug {
			return server.ListenAndServe()
		}
		return server.ListenAndServeTLS(cfg.SSLCertPath, cfg.SSLKeyPath)
	}, server.Shutdown, server.Close, shutdownTimeout)
}
