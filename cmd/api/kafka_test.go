package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestHTTPFailureStopsAndWaitsForConsumer(t *testing.T) {
	httpErr := errors.New("HTTP listener failed")
	closeErr := errors.New("reader close failed")
	started := make(chan struct{})
	closed := false
	err := runKafkaAndHTTP(context.Background(), func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		closed = true
		return closeErr
	}, func() error {
		<-started
		return httpErr
	}, func(context.Context) error { return nil }, func() error { return nil }, time.Second)
	if !closed || !errors.Is(err, httpErr) || !errors.Is(err, closeErr) {
		t.Fatalf("closed=%v error=%v", closed, err)
	}
}

func TestConsumerFailureReachesMain(t *testing.T) {
	workerErr := errors.New("consumer lifecycle failed")
	release := make(chan struct{})
	err := runKafkaAndHTTP(context.Background(), func(_ context.Context) error {
		return workerErr
	}, func() error {
		<-release
		return nil
	}, func(context.Context) error { close(release); return nil }, func() error { return nil }, time.Second)
	if !errors.Is(err, workerErr) {
		t.Fatalf("error=%v", err)
	}
}

func TestUnexpectedConsumerExit(t *testing.T) {
	release := make(chan struct{})
	err := runKafkaAndHTTP(context.Background(), func(_ context.Context) error {
		return nil
	}, func() error {
		<-release
		return nil
	}, func(context.Context) error { close(release); return nil }, func() error { return nil }, time.Second)
	if err == nil {
		t.Fatal("unexpected consumer exit must fail the process")
	}
}
