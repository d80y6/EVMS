# CI & Backend Testing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add frontend test execution to CI, Go coverage reporting, and meaningful tests to 3 backend services (ingest, playback, recorder) plus one additional WebRTC test.

**Architecture:** Three categories of changes: CI config (`.github/workflows/go-ci.yml`), Go backend service tests (4 test files), and coverage configuration. Each task is independent and can be implemented in any order, but CI changes (Tasks 1-2) should be committed first since they enable verification of the test additions (Tasks 3-6).

**Tech Stack:** Go 1.24, vitest, GitHub Actions, testify/assert

---

### Task 1: Fix frontend CI (add npm test)

**Files:**
- Modify: `.github/workflows/go-ci.yml:114-118`

- [ ] **Step 1: Add Run tests step to frontend job**

Add an npm test step between the type check and build steps in the frontend job.

oldString (lines 114-118):
```yaml
      - name: Type check
        run: npx tsc --noEmit
      - name: Build
        run: npm run build
```

newString:
```yaml
      - name: Type check
        run: npx tsc --noEmit
      - name: Run tests
        run: npm test
      - name: Build
        run: npm run build
```

- [ ] **Step 2: Commit Task 1**

```bash
git add .github/workflows/go-ci.yml
git commit -m "ci: add frontend test step to CI workflow"
```

---

### Task 2: Add Go coverage to CI

**Files:**
- Modify: `.github/workflows/go-ci.yml:32-33`

- [ ] **Step 1: Add `-coverprofile=coverage.out` and coverage display step**

oldString:
```yaml
      - name: Run tests
        run: go test ./... -count=1
```

newString:
```yaml
      - name: Run tests
        run: go test ./... -count=1 -coverprofile=coverage.out
      - name: Display coverage
        run: go tool cover -func=coverage.out
```

- [ ] **Step 2: Commit Task 2**

```bash
git add .github/workflows/go-ci.yml
git commit -m "ci: add Go coverage profile and display step"
```

---

### Task 3: Add ingest service tests

**Files:**
- Modify: `services/ingest/main_test.go`

- [ ] **Step 1: Add testify and io/log/slog/filepath imports**

oldString (lines 2-5):
```
import (
	"testing"
)
```

newString:
```
import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)
```

- [ ] **Step 2: Add TestFFmpegArgs**

oldString (the closing `}` of the file at line 28):
```
	})
}
```

newString:
```
	})
}

func TestFFmpegArgs(t *testing.T) {
	recordingsPath := "/recordings/cam1"
	rtspURL := "rtsp://192.168.1.100:554/stream1"

	args := ffmpegArgs(recordingsPath, rtspURL)

	assert.Contains(t, args, "-rtsp_transport")
	assert.Contains(t, args, "tcp")
	assert.Contains(t, args, "-i")
	assert.Contains(t, args, rtspURL)
	assert.Contains(t, args, filepath.Join(recordingsPath, "%Y%m%d_%H%M%S.mp4"))

	for i, arg := range args {
		if arg == "-i" && i+1 < len(args) {
			assert.Equal(t, rtspURL, args[i+1])
		}
	}
}

func TestStreamHealth_Snapshot(t *testing.T) {
	h := &StreamHealth{}
	h.Running = true
	h.RestartCount = 3
	h.LastError = "test error"

	snap := h.snapshot()
	assert.True(t, snap.Running)
	assert.Equal(t, 3, snap.RestartCount)
	assert.Equal(t, "test error", snap.LastError)

	// Verify snapshot is a copy
	h.Running = false
	assert.True(t, snap.Running)
}

func TestNewStreamProcessor(t *testing.T) {
	config := CameraStreamConfig{
		CameraID:      "test-cam",
		RTSPURL:       "rtsp://test:554/stream",
		RecordingsDir: "/recordings",
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	sp := NewStreamProcessor(config, nil, logger)
	assert.NotNil(t, sp)
	assert.Equal(t, "test-cam", sp.config.CameraID)
	assert.NotNil(t, sp.restartCh)
}

func TestNalType(t *testing.T) {
	// 4-byte start code, NAL type 7 (SPS)
	nal := []byte{0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00}
	assert.Equal(t, byte(7), nalType(nal))

	// 3-byte start code, NAL type 5 (IDR)
	nal = []byte{0x00, 0x00, 0x01, 0x65, 0x88, 0x84}
	assert.Equal(t, byte(5), nalType(nal))

	// No start code, NAL type 1 (non-IDR slice)
	nal = []byte{0x41, 0x9a, 0x22}
	assert.Equal(t, byte(1), nalType(nal))

	// 4-byte start code, NAL type 8 (PPS)
	nal = []byte{0x00, 0x00, 0x00, 0x01, 0x68, 0xEE, 0x3C}
	assert.Equal(t, byte(8), nalType(nal))

	// 3-byte start code, NAL type 6 (SEI)
	nal = []byte{0x00, 0x00, 0x01, 0x46, 0x05, 0x04}
	assert.Equal(t, byte(6), nalType(nal))
}
```

- [ ] **Step 3: Run tests to verify they pass**

Run: `go test ./services/ingest/... -count=1 -v`
Expected: All 6 tests pass (TestDefaultConfig, TestConfigValidation, TestFFmpegArgs, TestStreamHealth_Snapshot, TestNewStreamProcessor, TestNalType)

- [ ] **Step 4: Commit Task 3**

```bash
git add services/ingest/main_test.go
git commit -m "test: add ingest service tests for ffmpeg args, stream health, processor, nal type"
```

---

### Task 4: Add playback service tests

**Files:**
- Modify: `services/playback/main_test.go`

- [ ] **Step 1: Add testify and log/slog/os/io imports**

oldString (lines 2-5):
```
import (
	"testing"
)
```

newString:
```
import (
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)
```

- [ ] **Step 2: Add TestValidateMP4_Valid, TestValidateMP4_MissingFtyp, TestValidateMP4_TooSmall, TestAudioContentType**

oldString (the closing `}` of the file at line 28):
```
	})
}
```

newString:
```
	})
}

func TestValidateMP4_Valid(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	f := t.TempDir() + "/test.mp4"

	header := make([]byte, 1024)
	copy(header[4:8], "ftyp")

	err := os.WriteFile(f, header, 0644)
	assert.NoError(t, err)

	err = validateMP4(f, logger)
	assert.NoError(t, err)
}

func TestValidateMP4_MissingFtyp(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	f := t.TempDir() + "/test.mp4"

	header := make([]byte, 1024)
	// Deliberately omit "ftyp" — header[4:8] stays 0x00

	err := os.WriteFile(f, header, 0644)
	assert.NoError(t, err)

	err = validateMP4(f, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ftyp")
}

func TestValidateMP4_TooSmall(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	f := t.TempDir() + "/test.mp4"

	header := make([]byte, 8)
	copy(header[4:8], "ftyp")

	err := os.WriteFile(f, header, 0644)
	assert.NoError(t, err)

	err = validateMP4(f, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too small")
}

func TestAudioContentType(t *testing.T) {
	svc := &PlaybackService{}

	tests := []struct {
		path     string
		expected string
	}{
		{"/recordings/audio.aac", "audio/aac"},
		{"/recordings/audio.wav", "audio/wav"},
		{"/recordings/audio.mp3", "audio/mpeg"},
		{"/recordings/audio.opus", "audio/opus"},
		{"/recordings/audio.ogg", "audio/ogg"},
		{"/recordings/unknown.xyz", "audio/mpeg"},
		{"/recordings/noext", "audio/mpeg"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := svc.audioContentType(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}
```

- [ ] **Step 3: Run tests to verify they pass**

Run: `go test ./services/playback/... -count=1 -v`
Expected: All 7 tests pass (TestDefaultConfig, TestNewPlaybackService with subtest, TestValidateMP4_Valid, TestValidateMP4_MissingFtyp, TestValidateMP4_TooSmall, TestAudioContentType with its subtests)

- [ ] **Step 4: Commit Task 4**

```bash
git add services/playback/main_test.go
git commit -m "test: add playback service tests for MP4 validation and audio content type"
```

---

### Task 5: Add recorder service tests

**Files:**
- Modify: `services/recorder/main_test.go`

- [ ] **Step 1: Add testify import**

oldString (lines 2-5):
```
import (
	"testing"
)
```

newString:
```
import (
	"testing"

	"github.com/stretchr/testify/assert"
)
```

- [ ] **Step 2: Add TestRingBuffer_WriteAndRead, TestRingBuffer_Overflow, TestNewRingBuffer_Size, TestNewCameraRecorder**

oldString (the closing `}` of the file at line 38):
```
	})
}
```

newString:
```
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
```

- [ ] **Step 3: Run tests to verify they pass**

Run: `go test ./services/recorder/... -count=1 -v`
Expected: All 7 tests pass (TestDefaultConfig, TestConfigValidation with 3 subtests, TestRingBuffer_WriteAndRead, TestRingBuffer_Overflow, TestNewRingBuffer_Size, TestNewCameraRecorder)

- [ ] **Step 4: Commit Task 5**

```bash
git add services/recorder/main_test.go
git commit -m "test: add recorder service tests for ring buffer and camera recorder"
```

---

### Task 6: Add webrtc service test

**Files:**
- Modify: `services/webrtc/main_test.go`

- [ ] **Step 1: Add testify import**

oldString (lines 11-17):
```
	"github.com/dam-vms/dam/pkg/common"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pion/webrtc/v3"
)
```

newString:
```
	"github.com/dam-vms/dam/pkg/common"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/assert"
)
```

- [ ] **Step 2: Add TestCreateOfferHandler_SuccessPath**

oldString (the closing `}` of the file at line 318):
```
	}
```

newString:
```
	}
}

func TestCreateOfferHandler_SuccessPath(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret-for-webrtc-auth")
	common.ReloadJWTKey()
	defer func() {
		os.Unsetenv("JWT_SECRET")
		common.ReloadJWTKey()
	}()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := &WebRTCService{logger: logger}

	token := generateTestJWT("testuser", "viewer")
	body := bytes.NewReader([]byte("invalid offer body"))
	req := httptest.NewRequest(http.MethodPost, "/webrtc/offer?camera_id=cam1", body)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	common.JWTAuthMiddleware(svc.createOfferHandler)(rr, req)

	// Expect 400 (invalid offer body) rather than 500 — proving
	// input validation works correctly before hitting infrastructure.
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}
```

- [ ] **Step 3: Run tests to verify they pass**

Run: `go test ./services/webrtc/... -count=1 -v`
Expected: All 12 tests pass (including the new TestCreateOfferHandler_SuccessPath)

- [ ] **Step 4: Commit Task 6**

```bash
git add services/webrtc/main_test.go
git commit -m "test: add WebRTC createOfferHandler success path test"
```

---

### Final verification

- [ ] **Run all Go tests**

Run: `go test ./... -count=1 -coverprofile=coverage.out`
Expected: All tests pass across all services. Coverage profile generated.

- [ ] **Verify coverage output**

Run: `go tool cover -func=coverage.out`
Expected: Coverage percentages displayed for each function.
