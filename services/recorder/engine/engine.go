package recorder

import (
	"log"
	"time"
)

// Segment represents a recorded video segment
type Segment struct {
	StartTime time.Time
	EndTime   time.Time
	Path      string
	CameraID  string
}

// Engine handles the persistence of video segments
type Engine struct {
	StoragePath string
}

func (e *Engine) SaveSegment(seg Segment) error {
	log.Printf("Saving segment for camera %s: %s", seg.CameraID, seg.Path)
	// Implementation would include database indexing and file movement
	return nil
}
