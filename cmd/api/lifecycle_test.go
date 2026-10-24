package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSignalDrainsActiveRequestBeforeConsumerStops(t *testing.T) {
	entered, release, consumerStopped := make(chan struct{}), make(chan struct{}), make(chan struct{})
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		select {
		case <-consumerStopped:
			t.Error("consumer stopped during active request")
		default:
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	server.Start()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serveDone := make(chan struct{})
	draining := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- runKafkaAndHTTP(ctx, func(ctx context.Context) error {
			<-ctx.Done()
			close(consumerStopped)
			return nil
		}, func() error { <-serveDone; return http.ErrServerClosed }, func(ctx context.Context) error {
			close(draining)
			close(serveDone)
			return server.Config.Shutdown(ctx)
		}, server.Config.Close, time.Second)
	}()
	requestDone := make(chan error, 1)
	go func() {
		response, err := server.Client().Get(server.URL)
		if err == nil {
			if response.StatusCode != http.StatusAccepted {
				err = errors.New("unexpected status")
			}
			_ = response.Body.Close()
		}
		requestDone <- err
	}()
	<-entered
	cancel()
	<-draining
	close(release)
	if err := <-requestDone; err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	select {
	case <-consumerStopped:
	default:
		t.Fatal("consumer not stopped")
	}
}

func TestDrainTimeoutForcesCloseAndStillStopsConsumer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	forced := make(chan struct{})
	serverDone := make(chan struct{})
	err := runKafkaAndHTTP(ctx, func(ctx context.Context) error { <-ctx.Done(); return nil },
		func() error { <-serverDone; return http.ErrServerClosed },
		func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() },
		func() error { close(forced); close(serverDone); return nil }, 10*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error=%v", err)
	}
	select {
	case <-forced:
	default:
		t.Fatal("HTTP not force closed")
	}
}

func TestBlockedConsumerAndCloseAreBounded(t *testing.T) {
	for _, blockedConsumer := range []bool{true, false} {
		t.Run(map[bool]string{true: "consumer", false: "close"}[blockedConsumer], func(t *testing.T) {
			release := make(chan struct{})
			defer close(release)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if !blockedConsumer {
				err := bounded("close", 10*time.Millisecond, func(context.Context) error { <-release; return nil })
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("error=%v", err)
				}
				return
			}
			err := runKafkaAndHTTP(ctx, func(context.Context) error { <-release; return nil },
				func() error { return http.ErrServerClosed }, func(context.Context) error { return nil },
				func() error { return nil }, 10*time.Millisecond)
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestServerTimeouts(t *testing.T) {
	s := newHTTPServer(":0", http.NotFoundHandler())
	if s.ReadHeaderTimeout != 5*time.Second || s.ReadTimeout != 10*time.Second || s.WriteTimeout != 10*time.Second || s.IdleTimeout != time.Minute {
		t.Fatalf("unexpected server timeouts: %+v", s)
	}
}
