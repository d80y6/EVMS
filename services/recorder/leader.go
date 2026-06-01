package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	leaderBucket  = "recorder_leader"
	leaderKey     = "leader"
	leaderTTL     = 10 * time.Second
	heartbeatFreq = 3 * time.Second
)

type LeaderElection struct {
	nc        *nats.Conn
	kv        nats.KeyValue
	id        string
	mu        sync.RWMutex
	isLeader  bool
	leaderID  string
	stopCh    chan struct{}
	logger    *slog.Logger
}

func NewLeaderElection(nc *nats.Conn, id string) (*LeaderElection, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	kv, err := js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      leaderBucket,
		Description: "Recorder leader election",
		History:     5,
		TTL:         leaderTTL * 2,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create KV store: %w", err)
	}

	return &LeaderElection{
		nc:     nc,
		kv:     kv,
		id:     id,
		stopCh: make(chan struct{}),
		logger: slog.Default().With("component", "leader_election", "id", id),
	}, nil
}

func (l *LeaderElection) Start(ctx context.Context) {
	l.logger.Info("Starting leader election")

	l.elect()

	ticker := time.NewTicker(heartbeatFreq)
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
	_, err := l.kv.Create(leaderKey, []byte(l.id))
	if err != nil {
		entry, err := l.kv.Get(leaderKey)
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
	l.leaderID = l.id
	l.mu.Unlock()
	l.logger.Info("Elected as leader")
}

func (l *LeaderElection) renew() {
	_, err := l.kv.Put(leaderKey, []byte(l.id))
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
		if err := l.kv.Delete(leaderKey); err != nil {
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
	return l.id
}

func (l *LeaderElection) Stop() {
	close(l.stopCh)
}
