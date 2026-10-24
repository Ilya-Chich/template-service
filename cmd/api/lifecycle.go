package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	shutdownTimeout = 10 * time.Second
	requestTimeout  = 5 * time.Second
)

// A signal is normal only when every cause is cancellation. In particular,
// errors.Join from startup cleanup must not hide a separate Close failure.
func expectedCancellation(ctx context.Context, err error) bool {
	if ctx.Err() != context.Canceled || !errors.Is(err, context.Canceled) {
		return false
	}
	switch cause := err.(type) {
	case interface{ Unwrap() []error }:
		for _, child := range cause.Unwrap() {
			if !expectedCancellation(ctx, child) {
				return false
			}
		}
	case interface{ Unwrap() error }:
		return expectedCancellation(ctx, cause.Unwrap())
	}
	return true
}

func newHTTPServer(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr: address, Handler: withRequestDeadline(handler),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second,
	}
}

func withRequestDeadline(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		handler.ServeHTTP(w, r.WithContext(ctx))
	})
}

// bounded also bounds operations such as Close that do not accept a context.
// Their goroutine may outlive the phase; the process must exit on a timeout.
func bounded(name string, timeout time.Duration, operation func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- operation(ctx) }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%s: %w", name, ctx.Err())
	}
}

func runKafkaAndHTTP(parent context.Context, consume func(context.Context) error, serveHTTP func() error,
	shutdown func(context.Context) error, forceClose func() error, timeout time.Duration,
) (err error) {
	// Signals initiate draining; they must not cancel the consumer first.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	workerDone := make(chan error, 1)
	go func() { workerDone <- consume(ctx) }()
	httpDone := make(chan error, 1)
	go func() { httpDone <- serveHTTP() }()
	workerStopped, httpStopped := false, false
	select {
	case <-parent.Done():
	case err = <-httpDone:
		httpStopped = true
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
	case err = <-workerDone:
		workerStopped = true
		if err == nil {
			err = errors.New("kafka consumer exited unexpectedly")
		}
	}
	drainErr := bounded("HTTP shutdown", timeout, func(ctx context.Context) error {
		if shutdownErr := shutdown(ctx); shutdownErr != nil {
			return shutdownErr
		}
		if !httpStopped {
			select {
			case serveErr := <-httpDone:
				if !errors.Is(serveErr, http.ErrServerClosed) {
					return serveErr
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})
	var closed chan error
	if drainErr != nil {
		// Start force-close even if the drain exhausted its deadline. Waiting for
		// it shares the consumer phase's bound rather than adding another phase.
		closed = make(chan error, 1)
		go func() { closed <- forceClose() }()
	}
	cancel()
	err = errors.Join(err, drainErr, bounded("consumer shutdown", timeout, func(ctx context.Context) error {
		var closeErr error
		if closed != nil {
			select {
			case closeErr = <-closed:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if workerStopped {
			return closeErr
		}
		select {
		case workerErr := <-workerDone:
			return errors.Join(closeErr, workerErr)
		case <-ctx.Done():
			return errors.Join(closeErr, ctx.Err())
		}
	}))
	return err
}
