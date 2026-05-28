use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use uuid::Uuid;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StreamSegment {
    pub id: String,
    pub stream_id: String,
    pub start_time_ms: i64,
    pub end_time_ms: i64,
    pub uri: String,
    pub duration_ms: i64,
    pub created_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct VirtualSegment {
    pub id: String,
    pub segments: Vec<StreamSegment>,
    pub start_time_ms: i64,
    pub end_time_ms: i64,
    pub aligned: bool,
}

pub struct SegmentManager {
    segments: HashMap<String, StreamSegment>,
    by_stream: HashMap<String, Vec<String>>,
}

impl SegmentManager {
    pub fn new() -> Self {
        SegmentManager {
            segments: HashMap::new(),
            by_stream: HashMap::new(),
        }
    }

    pub fn add_segment(&mut self, segment: StreamSegment) -> Result<(), String> {
        if self.segments.contains_key(&segment.id) {
            return Err(format!("Segment {} already exists", segment.id));
        }
        
        let stream_id = segment.stream_id.clone();
        self.segments.insert(segment.id.clone(), segment);
        
        self.by_stream.entry(stream_id).or_insert_with(Vec::new).push(segment.id.clone());
        
        Ok(())
    }

    pub fn get_segment(&self, id: &str) -> Option<&StreamSegment> {
        self.segments.get(id)
    }

    pub fn get_segments_by_stream(&self, stream_id: &str) -> Vec<&StreamSegment> {
        self.by_stream.get(stream_id)
            .map(|ids| ids.iter().filter_map(|id| self.segments.get(id)).collect())
            .unwrap_or_default()
    }

    pub fn create_virtual_segment(&self, stream_ids: &[String], start_ms: i64, end_ms: i64) -> Option<VirtualSegment> {
        let mut segments = Vec::new();
        
        for stream_id in stream_ids {
            for segment in self.get_segments_by_stream(stream_id) {
                if segment.start_time_ms <= end_ms && segment.end_time_ms >= start_ms {
                    segments.push(segment.clone());
                }
            }
        }

        if segments.is_empty() {
            return None;
        }

        segments.sort_by_key(|s| s.start_time_ms);

        Some(VirtualSegment {
            id: Uuid::new_v4().to_string(),
            segments,
            start_time_ms: start_ms,
            end_time_ms: end_ms,
            aligned: true,
        })
    }

    pub fn find_gaps(&self, stream_id: &str, max_gap_ms: i64) -> Vec<(i64, i64)> {
        let segments = self.get_segments_by_stream(stream_id);
        if segments.is_empty() {
            return vec![];
        }

        let mut sorted: Vec<&StreamSegment> = segments;
        sorted.sort_by_key(|s| s.start_time_ms);

        let mut gaps = Vec::new();
        let mut last_end = sorted[0].start_time_ms;

        for segment in sorted {
            if segment.start_time_ms - last_end > max_gap_ms {
                gaps.push((last_end, segment.start_time_ms));
            }
            last_end = segment.end_time_ms;
        }

        gaps
    }
}

impl Default for SegmentManager {
    fn default() -> Self {
        Self::new()
    }
}
