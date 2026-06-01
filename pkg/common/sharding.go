package common

import (
	"hash/fnv"
	"os"
	"strconv"
	"strings"
)

type ShardConfig struct {
	TotalShards int
	ShardIndex  int
}

func ShardConfigFromEnv() ShardConfig {
	totalShards := 1
	shardIndex := 0

	if s := os.Getenv("RECORDER_TOTAL_SHARDS"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			totalShards = v
		}
	}

	if s := os.Getenv("RECORDER_SHARD_INDEX"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v >= 0 && v < totalShards {
			shardIndex = v
		}
	} else if totalShards > 1 {
		hostname, _ := os.Hostname()
		if lastDash := strings.LastIndex(hostname, "-"); lastDash >= 0 {
			if v, err := strconv.Atoi(hostname[lastDash+1:]); err == nil {
				shardIndex = v % totalShards
			}
		}
	}

	return ShardConfig{
		TotalShards: totalShards,
		ShardIndex:  shardIndex,
	}
}

func (s ShardConfig) IsSharded() bool {
	return s.TotalShards > 1
}

func (s ShardConfig) OwnsCamera(cameraID string) bool {
	if !s.IsSharded() {
		return true
	}
	return s.ShardForCamera(cameraID) == s.ShardIndex
}

func (s ShardConfig) ShardForCamera(cameraID string) int {
	h := fnv.New32a()
	h.Write([]byte(cameraID))
	return int(h.Sum32() % uint32(s.TotalShards))
}
