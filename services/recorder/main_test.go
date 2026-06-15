package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultRecorderConfig()
	if config.RetentionDays != 7 {
		t.Errorf("default RetentionDays = %d, want 7", config.RetentionDays)
	}
	if config.MetricsAddr != ":2112" {
		t.Errorf("default MetricsAddr = %q, want %q", config.MetricsAddr, ":2112")
	}
}

func TestConfigValidation(t *testing.T) {
	t.Run("empty DB URL fails", func(t *testing.T) {
		config := RecorderConfig{DBURL: "", NATSURL: "nats://nats:4222"}
		if err := config.Validate(); err == nil {
			t.Error("expected validation error with empty DBURL")
		}
	})

	t.Run("empty NATS URL fails", func(t *testing.T) {
		config := RecorderConfig{DBURL: "postgres://localhost", NATSURL: ""}
		if err := config.Validate(); err == nil {
			t.Error("expected validation error with empty NATSURL")
		}
	})

	t.Run("valid config passes", func(t *testing.T) {
		config := RecorderConfig{DBURL: "postgres://localhost", NATSURL: "nats://nats:4222"}
		if err := config.Validate(); err != nil {
			t.Errorf("unexpected validation error: %v", err)
		}
	})
}

func TestRingBuffer_WriteAndRead(t *testing.T) {
	rb := newRingBuffer(1, 1024) // 1 second * 1024 Kbps / 8 = 128 KB

	data := []byte("hello world")
	n, err := rb.Write(data)
	assert.NoError(t, err)
	assert.Equal(t, len(data), n)

	out := rb.Bytes()
	assert.Equal(t, data, out)
}

func TestRingBuffer_Overflow(t *testing.T) {
	rb := newRingBuffer(1, 8) // 1 second * 8 Kbps / 8 = 1 KB capacity

	largeData := make([]byte, 2048)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	n, err := rb.Write(largeData)
	assert.NoError(t, err)
	assert.Equal(t, 2048, n)

	out := rb.Bytes()
	// With 1024 capacity and 2048 writes, only the last 1024 bytes remain
	assert.Equal(t, 1024, len(out))
	assert.Equal(t, largeData[1024:], out)
}

func TestNewRingBuffer_Size(t *testing.T) {
	rb := newRingBuffer(5, 4096) // 5 * 4096 * 1024 / 8 = 2,621,440
	assert.Equal(t, 5*4096*1024/8, rb.capacity)
	assert.NotNil(t, rb.data)
	assert.Equal(t, rb.capacity, len(rb.data))
}

func TestNewCameraRecorder(t *testing.T) {
	cr := NewCameraRecorder("cam-1", 5, 4096)
	assert.NotNil(t, cr)
	assert.Equal(t, "cam-1", cr.cameraID)
	assert.NotNil(t, cr.buf)
	assert.Equal(t, 5*4096*1024/8, cr.buf.capacity)
}
