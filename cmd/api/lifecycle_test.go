package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	httpadapter "http-api-template/internal/adapter/http"
	logadapter "http-api-template/internal/adapter/logger"
	"http-api-template/internal/core/domain"
	portmocks "http-api-template/internal/mocks/port"

	"github.com/stretchr/testify/mock"
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
	if s.ReadHeaderTimeout != 5*time.Second || s.ReadTimeout != 10*time.Second || s.WriteTimeout != 10*time.Second ||
		s.IdleTimeout != time.Minute {
		t.Fatalf("unexpected server timeouts: %+v", s)
	}
}

func TestRequestDeadline(t *testing.T) {
	type contextKey struct{}
	for _, earlier := range []bool{false, true} {
		t.Run(map[bool]string{false: "server budget", true: "earlier parent deadline"}[earlier], func(t *testing.T) {
			parent := context.WithValue(context.Background(), contextKey{}, "value")
			if earlier {
				var cancel context.CancelFunc
				parent, cancel = context.WithTimeout(parent, time.Second)
				defer cancel()
			}
			before := time.Now()
			var received context.Context
			server := newHTTPServer(":0", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received = r.Context()
				deadline, ok := received.Deadline()
				if !ok {
					t.Fatal("request has no deadline")
				}
				if earlier {
					want, _ := parent.Deadline()
					if !deadline.Equal(want) {
						t.Fatalf("deadline=%v, want %v", deadline, want)
					}
				} else if deadline.Before(before.Add(requestTimeout)) || deadline.After(time.Now().Add(requestTimeout)) {
					t.Fatalf("unexpected deadline: %v", deadline)
				}
				if received.Value(contextKey{}) != "value" {
					t.Fatal("context value lost")
				}
				w.WriteHeader(http.StatusAccepted)
			}))
			response := httptest.NewRecorder()
			server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(parent))
			if response.Code != http.StatusAccepted {
				t.Fatalf("status=%d", response.Code)
			}
			if received.Err() != context.Canceled {
				t.Fatalf("context after handler: %v", received.Err())
			}
			if parent.Err() != nil {
				t.Fatalf("parent canceled: %v", parent.Err())
			}
		})
	}
}

func TestRequestPreservesParentCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	handler := withRequestDeadline(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		cancel()
		<-r.Context().Done()
		if r.Context().Err() != context.Canceled {
			t.Fatalf("error=%v", r.Context().Err())
		}
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil).WithContext(parent))
}

func TestRequestDeadlineReachesService(t *testing.T) {
	dummies := portmocks.NewMockDummyService(t)
	dummies.EXPECT().Get(mock.Anything, deadlineDummyID).
		RunAndReturn(func(ctx context.Context, _ string) (domain.Dummy, error) {
			if _, ok := ctx.Deadline(); !ok {
				t.Error("service received no deadline")
			}
			<-ctx.Done()
			return domain.Dummy{}, fmt.Errorf("dependency: %w", ctx.Err())
		}).Once()
	ids := portmocks.NewMockIDGenerator(t)
	router, err := httpadapter.NewRouter(dummies, nil, nil, logadapter.New(&bytes.Buffer{}, ids), ids)
	if err != nil {
		t.Fatal(err)
	}
	parent, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/dummies/"+deadlineDummyID, nil).WithContext(parent)
	request.Header.Set("X-Request-ID", "deadline-test")
	response := httptest.NewRecorder()
	newHTTPServer(":0", router).Handler.ServeHTTP(response, request)
	if response.Code != http.StatusGatewayTimeout || response.Body.Len() != 0 {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
}

const deadlineDummyID = "550e8400-e29b-41d4-a716-446655440000"
