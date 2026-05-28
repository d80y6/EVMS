use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StreamOffset {
    pub stream_id: String,
    pub offset_ms: f64,
    pub confidence: f32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AlignmentPlan {
    pub streams: Vec<StreamOffset>,
    pub reference_stream: String,
    pub created_at: i64,
}

pub struct StreamAligner {
    offsets: HashMap<String, f64>,
    reference_stream: Option<String>,
}

impl StreamAligner {
    pub fn new() -> Self {
        StreamAligner {
            offsets: HashMap::new(),
            reference_stream: None,
        }
    }

    pub fn set_offset(&mut self, stream_id: &str, offset_ms: f64) {
        self.offsets.insert(stream_id.to_string(), offset_ms);
    }

    pub fn set_reference(&mut self, stream_id: &str) {
        self.reference_stream = Some(stream_id.to_string());
    }

    pub fn get_alignment_plan(&self) -> Option<AlignmentPlan> {
        let reference = self.reference_stream.clone()?;
        
        let ref_offset = self.offsets.get(&reference).copied().unwrap_or(0.0);
        
        let streams: Vec<StreamOffset> = self.offsets.iter().map(|(id, offset)| {
            StreamOffset {
                stream_id: id.clone(),
                offset_ms: offset - ref_offset,
                confidence: 1.0,
            }
        }).collect();

        Some(AlignmentPlan {
            streams,
            reference_stream: reference,
            created_at: chrono::Utc::now().timestamp_millis(),
        })
    }

    pub fn calculate_cross_correlation(&self, signal_a: &[f32], signal_b: &[f32]) -> (i32, f32) {
        if signal_a.len() != signal_b.len() || signal_a.is_empty() {
            return (0, 0.0);
        }

        let n = signal_a.len();
        let mut best_lag = 0i32;
        let mut best_corr = f32::NEG_INFINITY;

        let max_lag = (n / 10).min(1000);

        for lag in -max_lag..=max_lag {
            let mut corr = 0.0f32;
            let mut count = 0u32;

            for i in 0..n {
                let j = i as i32 + lag;
                if j >= 0 && (j as usize) < n {
                    corr += signal_a[i] * signal_b[j as usize];
                    count += 1;
                }
            }

            if count > 0 {
                corr /= count as f32;
                if corr > best_corr {
                    best_corr = corr;
                    best_lag = lag;
                }
            }
        }

        (best_lag, best_corr)
    }
}

impl Default for StreamAligner {
    fn default() -> Self {
        Self::new()
    }
}
