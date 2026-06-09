package common

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	DefaultLeaderBucket = "service_leader"
	DefaultLeaderKey    = "leader"
	DefaultLeaderTTL    = 20 * time.Second
	DefaultHeartbeat    = 3 * time.Second
)

type LeaderElection struct {
	kv        nats.KeyValue
	bucket    string
	key       string
	nodeID    string
	ttl       time.Duration
	heartbeat time.Duration
	logger    *slog.Logger
	stopCh    chan struct{}

	mu       sync.RWMutex
	leaderID string
	isLeader bool
}

type LeaderOption func(*LeaderElection)

func WithLeaderBucket(bucket string) LeaderOption {
	return func(l *LeaderElection) {
		l.bucket = bucket
	}
}

func WithLeaderKey(key string) LeaderOption {
	return func(l *LeaderElection) {
		l.key = key
	}
}

func WithLeaderTTL(ttl time.Duration) LeaderOption {
	return func(l *LeaderElection) {
		l.ttl = ttl
	}
}

func WithLeaderHeartbeat(d time.Duration) LeaderOption {
	return func(l *LeaderElection) {
		l.heartbeat = d
	}
}

func WithLeaderLogger(logger *slog.Logger) LeaderOption {
	return func(l *LeaderElection) {
		l.logger = logger
	}
}

func NewLeaderElection(nc *nats.Conn, nodeID string, opts ...LeaderOption) (*LeaderElection, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	l := &LeaderElection{
		bucket:    DefaultLeaderBucket,
		key:       DefaultLeaderKey,
		nodeID:    nodeID,
		ttl:       DefaultLeaderTTL,
		heartbeat: DefaultHeartbeat,
		logger:    slog.Default().With("component", "leader_election", "id", nodeID),
		stopCh:    make(chan struct{}),
	}

	for _, opt := range opts {
		opt(l)
	}

	kv, err := js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      l.bucket,
		Description: "Service leader election",
		History:     5,
		TTL:         l.ttl,
	})
	if err != nil {
		kv, err = js.KeyValue(l.bucket)
		if err != nil {
			return nil, fmt.Errorf("failed to create or open KV store: %w", err)
		}
	}

	l.kv = kv
	return l, nil
}

func (l *LeaderElection) Start(ctx context.Context) {
	l.logger.Info("Starting leader election")

	l.elect()

	ticker := time.NewTicker(l.heartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			l.logger.Info("Leader election stopping")
			l.release()
			return
		case <-l.stopCh:
			l.release()
			return
		case <-ticker.C:
			if l.IsLeader() {
				l.renew()
			} else {
				l.elect()
			}
		}
	}
}

func (l *LeaderElection) elect() {
	_, err := l.kv.Create(l.key, []byte(l.nodeID))
	if err != nil {
		entry, err := l.kv.Get(l.key)
		if err != nil {
			l.logger.Debug("Failed to get leader key", "error", err)
			return
		}
		leaderID := string(entry.Value())

		l.mu.Lock()
		l.isLeader = false
		l.leaderID = leaderID
		l.mu.Unlock()
		return
	}

	l.mu.Lock()
	l.isLeader = true
	l.leaderID = l.nodeID
	l.mu.Unlock()
	l.logger.Info("Elected as leader")
}

func (l *LeaderElection) renew() {
	_, err := l.kv.Put(l.key, []byte(l.nodeID))
	if err != nil {
		l.logger.Error("Failed to renew leadership", "error", err)
		l.mu.Lock()
		l.isLeader = false
		l.mu.Unlock()
	}
}

func (l *LeaderElection) release() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.isLeader {
		if err := l.kv.Delete(l.key); err != nil {
			l.logger.Error("Failed to delete leader key", "error", err)
		}
		l.isLeader = false
		l.logger.Info("Released leadership")
	}
}

func (l *LeaderElection) IsLeader() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.isLeader
}

func (l *LeaderElection) LeaderID() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.leaderID
}

func (l *LeaderElection) ID() string {
	return l.nodeID
}

func (l *LeaderElection) Stop() {
	close(l.stopCh)
}
