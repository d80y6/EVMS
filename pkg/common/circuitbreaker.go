package common

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
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

func ConnectNATSWithCircuitBreaker(url string, cb *gobreaker.CircuitBreaker, opts ...nats.Option) (*nats.Conn, error) {
	defaultOpts := []nats.Option{
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2 * time.Second),
	}
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
