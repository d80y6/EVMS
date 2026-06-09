package main

import (
	"context"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/nats-io/nats.go"
)

const (
	leaderBucket  = "recorder_leader"
	leaderKey     = "leader"
	leaderTTL     = 10 * time.Second
	heartbeatFreq = 3 * time.Second
)

type LeaderElection struct {
	impl *common.LeaderElection
}

func NewLeaderElection(nc *nats.Conn, id string) (*LeaderElection, error) {
	impl, err := common.NewLeaderElection(nc, id,
		common.WithLeaderBucket(leaderBucket),
		common.WithLeaderKey(leaderKey),
		common.WithLeaderTTL(leaderTTL*2),
		common.WithLeaderHeartbeat(heartbeatFreq),
	)
	if err != nil {
		return nil, err
	}
	return &LeaderElection{impl: impl}, nil
}

func (l *LeaderElection) Start(ctx context.Context) { l.impl.Start(ctx) }
func (l *LeaderElection) Stop()                     { l.impl.Stop() }
func (l *LeaderElection) IsLeader() bool            { return l.impl.IsLeader() }
func (l *LeaderElection) LeaderID() string          { return l.impl.LeaderID() }
func (l *LeaderElection) ID() string                { return l.impl.ID() }
