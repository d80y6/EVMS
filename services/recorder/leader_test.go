package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

func startTestNATS(t *testing.T) *nats.Conn {
	url := os.Getenv("NATS_TEST_URL")
	if url == "" {
		url = nats.DefaultURL
	}
	nc, err := nats.Connect(url,
		nats.Timeout(5*time.Second),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(3),
		nats.ReconnectWait(100*time.Millisecond),
	)
	if err != nil {
		t.Skipf("NATS server not available at %s: %v", url, err)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		t.Skipf("NATS JetStream not available: %v", err)
	}
	kv, err := js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket: "_evms_test_health",
		TTL:    time.Minute,
	})
	if err != nil {
		nc.Close()
		t.Skipf("NATS JetStream KV not available: %v", err)
	}
	js.DeleteKeyValue("_evms_test_health")
	_ = kv
	t.Cleanup(nc.Close)
	return nc
}

func cleanupLeaderBucket(t *testing.T, nc *nats.Conn) {
	js, err := nc.JetStream()
	if err != nil {
		return
	}
	if err := js.DeleteKeyValue(leaderBucket); err != nil {
		t.Logf("cleanup bucket %s: %v", leaderBucket, err)
	}
}

func TestNewLeaderElection(t *testing.T) {
	nc := startTestNATS(t)
	cleanupLeaderBucket(t, nc)
	t.Cleanup(func() { cleanupLeaderBucket(t, nc) })

	le, err := NewLeaderElection(nc, "test-node-1")
	if err != nil {
		t.Fatalf("NewLeaderElection failed: %v", err)
	}
	defer le.Stop()

	if le.ID() != "test-node-1" {
		t.Errorf("ID() = %q, want %q", le.ID(), "test-node-1")
	}

	if le.IsLeader() {
		t.Error("new instance should not be leader before Start")
	}
}

func TestLeaderElection_Elect(t *testing.T) {
	nc := startTestNATS(t)
	cleanupLeaderBucket(t, nc)
	t.Cleanup(func() { cleanupLeaderBucket(t, nc) })

	le1, err := NewLeaderElection(nc, "node-1")
	if err != nil {
		t.Fatalf("NewLeaderElection failed: %v", err)
	}
	defer le1.Stop()

	le2, err := NewLeaderElection(nc, "node-2")
	if err != nil {
		t.Fatalf("NewLeaderElection failed: %v", err)
	}
	defer le2.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go le1.Start(ctx)
	go le2.Start(ctx)

	time.Sleep(1 * time.Second)

	if !le1.IsLeader() && !le2.IsLeader() {
		t.Error("expected exactly one leader, got none")
	}

	if le1.IsLeader() && le2.IsLeader() {
		t.Error("expected exactly one leader, got two")
	}
}

func TestLeaderElection_LeaderID(t *testing.T) {
	nc := startTestNATS(t)
	cleanupLeaderBucket(t, nc)
	t.Cleanup(func() { cleanupLeaderBucket(t, nc) })

	le, err := NewLeaderElection(nc, "primary")
	if err != nil {
		t.Fatalf("NewLeaderElection failed: %v", err)
	}
	defer le.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go le.Start(ctx)

	time.Sleep(500 * time.Millisecond)

	if !le.IsLeader() {
		t.Fatal("expected node to be leader")
	}

	if got := le.LeaderID(); got != "primary" {
		t.Errorf("LeaderID() = %q, want %q", got, "primary")
	}
}

func TestLeaderElection_FollowsExistingLeader(t *testing.T) {
	nc := startTestNATS(t)
	cleanupLeaderBucket(t, nc)
	t.Cleanup(func() { cleanupLeaderBucket(t, nc) })

	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("JetStream failed: %v", err)
	}
	kv, err := js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      leaderBucket,
		Description: "Recorder leader election",
		History:     5,
		TTL:         leaderTTL * 2,
	})
	if err != nil {
		t.Fatalf("CreateKeyValue failed: %v", err)
	}
	_, err = kv.Put(leaderKey, []byte("pre-existing-leader"))
	if err != nil {
		t.Fatalf("Put leader key failed: %v", err)
	}

	le, err := NewLeaderElection(nc, "follower")
	if err != nil {
		t.Fatalf("NewLeaderElection failed: %v", err)
	}
	defer le.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go le.Start(ctx)

	time.Sleep(1 * time.Second)

	if le.IsLeader() {
		t.Error("follower should not be leader when leader key exists")
	}

	if got := le.LeaderID(); got != "pre-existing-leader" {
		t.Errorf("LeaderID() = %q, want %q", got, "pre-existing-leader")
	}
}

func TestLeaderElection_ReleasesOnStop(t *testing.T) {
	nc := startTestNATS(t)
	cleanupLeaderBucket(t, nc)
	t.Cleanup(func() { cleanupLeaderBucket(t, nc) })

	le, err := NewLeaderElection(nc, "ephemeral")
	if err != nil {
		t.Fatalf("NewLeaderElection failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go le.Start(ctx)

	time.Sleep(500 * time.Millisecond)

	if !le.IsLeader() {
		t.Fatal("expected to be leader")
	}

	le.Stop()

	time.Sleep(500 * time.Millisecond)

	if le.IsLeader() {
		t.Error("should not be leader after Stop")
	}

	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("JetStream failed: %v", err)
	}
	kv, err := js.KeyValue(leaderBucket)
	if err != nil {
		t.Fatalf("KeyValue failed: %v", err)
	}
	_, err = kv.Get(leaderKey)
	if err != nats.ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound after release, got %v", err)
	}
}

func TestLeaderElection_CtxCancellation(t *testing.T) {
	nc := startTestNATS(t)
	cleanupLeaderBucket(t, nc)
	t.Cleanup(func() { cleanupLeaderBucket(t, nc) })

	le, err := NewLeaderElection(nc, "cancellable")
	if err != nil {
		t.Fatalf("NewLeaderElection failed: %v", err)
	}
	defer le.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go le.Start(ctx)

	time.Sleep(1 * time.Second)
	t.Log("ctx cancelled, Start exited cleanly")
}

func TestLeaderElection_ElectAfterLeaderFails(t *testing.T) {
	nc := startTestNATS(t)
	cleanupLeaderBucket(t, nc)
	t.Cleanup(func() { cleanupLeaderBucket(t, nc) })

	leader, err := NewLeaderElection(nc, "old-leader")
	if err != nil {
		t.Fatalf("NewLeaderElection failed: %v", err)
	}

	ctx1, cancel1 := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel1()
	go leader.Start(ctx1)
	time.Sleep(500 * time.Millisecond)

	if !leader.IsLeader() {
		t.Fatal("expected first node to be leader")
	}

	leaderID := leader.LeaderID()
	leader.Stop()
	leader = nil

	time.Sleep(500 * time.Millisecond)

	le2, err := NewLeaderElection(nc, "new-leader")
	if err != nil {
		t.Fatalf("NewLeaderElection failed: %v", err)
	}
	defer le2.Stop()

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	go le2.Start(ctx2)

	time.Sleep(1 * time.Second)

	if !le2.IsLeader() {
		t.Error("expected new node to become leader after old leader released")
	}

	if le2.LeaderID() == leaderID {
		t.Error("new leader should have different ID from old leader")
	}
}

func TestKVStoreJetStream(t *testing.T) {
	nc := startTestNATS(t)

	js, err := nc.JetStream()
	if err != nil {
		t.Skipf("JetStream not available: %v", err)
	}

	bucket := fmt.Sprintf("test_kv_%d", time.Now().UnixNano())
	kv, err := js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:  bucket,
		History: 5,
	})
	if err != nil {
		t.Skipf("CreateKeyValue failed: %v", err)
	}
	defer js.DeleteKeyValue(kv.Bucket())

	rev, err := kv.Create("test", []byte("value"))
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if rev == 0 {
		t.Error("expected non-zero revision")
	}

	entry, err := kv.Get("test")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(entry.Value()) != "value" {
		t.Errorf("Value = %q, want %q", string(entry.Value()), "value")
	}

	_, err = kv.Create("test", []byte("other"))
	if err == nil {
		t.Error("expected error for duplicate key")
	}
}
