package common

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nats-io/nats.go"
	"github.com/sony/gobreaker"
)

var (
	ErrCircuitOpen = errors.New("circuit breaker is open")
)

func NewDBCircuitBreaker(name string) *gobreaker.CircuitBreaker {
	return gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        name + "-db",
		MaxRequests: 3,
		Interval:    30 * time.Second,
		Timeout:     60 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 5 && failureRatio >= 0.6
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			slog.Warn("circuit breaker state changed",
				"name", name, "from", from.String(), "to", to.String())
		},
	})
}

func NewNATSCircuitBreaker(name string) *gobreaker.CircuitBreaker {
	return gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        name + "-nats",
		MaxRequests: 5,
		Interval:    20 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.TotalFailures >= 3
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			slog.Warn("nats circuit breaker state changed",
				"name", name, "from", from.String(), "to", to.String())
		},
	})
}

func ConnectDBWithCircuitBreaker(ctx context.Context, driverName, dataSourceName string, cb *gobreaker.CircuitBreaker) (*sqlx.DB, error) {
	result, err := cb.Execute(func() (interface{}, error) {
		db, err := sqlx.ConnectContext(ctx, driverName, dataSourceName)
		if err != nil {
			return nil, fmt.Errorf("db connect failed: %w", err)
		}
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(10)
		db.SetConnMaxLifetime(5 * time.Minute)
		db.SetConnMaxIdleTime(1 * time.Minute)
		return db, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*sqlx.DB), nil
}

func NATSTLSOptions() []nats.Option {
	certFile := os.Getenv("NATS_TLS_CERT")
	keyFile := os.Getenv("NATS_TLS_KEY")
	caFile := os.Getenv("NATS_TLS_CA")
	if certFile == "" && keyFile == "" && caFile == "" {
		return nil
	}
	var opts []nats.Option
	if caFile != "" {
		opts = append(opts, nats.RootCAs(caFile))
		slog.Info("NATS TLS CA configured", "file", caFile)
	}
	if certFile != "" && keyFile != "" {
		opts = append(opts, nats.ClientCert(certFile, keyFile))
		slog.Info("NATS TLS client cert configured", "cert", certFile)
	}
	if len(opts) > 0 {
		opts = append(opts, nats.Secure())
	}
	return opts
}

func ConnectNATSWithCircuitBreaker(url string, cb *gobreaker.CircuitBreaker, opts ...nats.Option) (*nats.Conn, error) {
	defaultOpts := []nats.Option{
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2 * time.Second),
	}
	tlsOpts := NATSTLSOptions()
	defaultOpts = append(defaultOpts, tlsOpts...)
	allOpts := append(defaultOpts, opts...)

	result, err := cb.Execute(func() (interface{}, error) {
		nc, err := nats.Connect(url, allOpts...)
		if err != nil {
			return nil, fmt.Errorf("nats connect failed: %w", err)
		}
		return nc, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*nats.Conn), nil
}

func NewHTTPCircuitBreaker(name string) *gobreaker.CircuitBreaker {
	return gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        name + "-http",
		MaxRequests: 10,
		Interval:    30 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.Requests >= 5 && float64(counts.TotalFailures)/float64(counts.Requests) >= 0.5
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			slog.Warn("http circuit breaker state changed",
				"name", name, "from", from.String(), "to", to.String())
		},
	})
}

// CircuitBreakerTransport wraps an http.RoundTripper with circuit breaker and retry logic.
type CircuitBreakerTransport struct {
	cb       *gobreaker.CircuitBreaker
	base     http.RoundTripper
	timeout  time.Duration
	retries  int
}

func NewCircuitBreakerTransport(cb *gobreaker.CircuitBreaker, timeout time.Duration, retries int, base http.RoundTripper) *CircuitBreakerTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &CircuitBreakerTransport{
		cb:      cb,
		base:    base,
		timeout: timeout,
		retries: retries,
	}
}

func (t *CircuitBreakerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= t.retries; attempt++ {
		result, err := t.cb.Execute(func() (interface{}, error) {
			if t.timeout > 0 {
				ctx, cancel := context.WithTimeout(req.Context(), t.timeout)
				defer cancel()
				req = req.WithContext(ctx)
			}
			resp, err := t.base.RoundTrip(req)
			if err != nil {
				return nil, err
			}
			if resp.StatusCode >= 500 {
				resp.Body.Close()
				return nil, fmt.Errorf("upstream %d", resp.StatusCode)
			}
			return resp, nil
		})
		if err == nil {
			return result.(*http.Response), nil
		}
		lastErr = err
		if attempt < t.retries {
			backoff := time.Duration(100*attempt*attempt) * time.Millisecond
			if backoff < 100*time.Millisecond {
				backoff = 100 * time.Millisecond
			}
			if backoff > 2*time.Second {
				backoff = 2 * time.Second
			}
			time.Sleep(backoff)
		}
	}
	return nil, fmt.Errorf("upstream request failed after %d retries: %w", t.retries, lastErr)
}

func WrapDB(db *sqlx.DB, cb *gobreaker.CircuitBreaker) *CircuitBreakerDB {
	return &CircuitBreakerDB{db: db, cb: cb}
}

type CircuitBreakerDB struct {
	db *sqlx.DB
	cb *gobreaker.CircuitBreaker
}

func (c *CircuitBreakerDB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	result, err := c.cb.Execute(func() (interface{}, error) {
		return c.db.ExecContext(ctx, query, args...)
	})
	if err != nil {
		return nil, err
	}
	return result.(sql.Result), nil
}

func (c *CircuitBreakerDB) NamedExecContext(ctx context.Context, query string, arg interface{}) (sql.Result, error) {
	result, err := c.cb.Execute(func() (interface{}, error) {
		return c.db.NamedExecContext(ctx, query, arg)
	})
	if err != nil {
		return nil, err
	}
	return result.(sql.Result), nil
}

func (c *CircuitBreakerDB) SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	_, err := c.cb.Execute(func() (interface{}, error) {
		return nil, c.db.SelectContext(ctx, dest, query, args...)
	})
	return err
}

func (c *CircuitBreakerDB) GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	_, err := c.cb.Execute(func() (interface{}, error) {
		return nil, c.db.GetContext(ctx, dest, query, args...)
	})
	return err
}

func (c *CircuitBreakerDB) DB() *sqlx.DB {
	return c.db
}

func (c *CircuitBreakerDB) Close() error {
	return c.db.Close()
}
