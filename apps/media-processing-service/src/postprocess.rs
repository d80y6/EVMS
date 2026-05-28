use crate::error::{Result, MediaProcessingError};
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Detection {
    pub label: i32,
    pub confidence: f32,
    pub bbox: BoundingBox,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BoundingBox {
    pub x_min: f32,
    pub y_min: f32,
    pub x_max: f32,
    pub y_max: f32,
}

pub fn non_max_suppression(detections: &mut Vec<Detection>, iou_threshold: f32) -> Vec<Detection> {
    if detections.is_empty() {
        return Vec::new();
    }

    detections.sort_by(|a, b| b.confidence.partial_cmp(&a.confidence).unwrap());

    let mut keep = Vec::new();
    let mut suppressed = vec![false; detections.len()];

    for i in 0..detections.len() {
        if suppressed[i] {
            continue;
        }
        keep.push(detections[i].clone());

        for j in (i + 1)..detections.len() {
            if suppressed[j] {
                continue;
            }

            let iou = calculate_iou(&detections[i].bbox, &detections[j].bbox);
            if iou > iou_threshold {
                suppressed[j] = true;
            }
        }
    }

    keep
}

fn calculate_iou(a: &BoundingBox, b: &BoundingBox) -> f32 {
    let x_min = a.x_min.max(b.x_min);
    let y_min = a.y_min.max(b.y_min);
    let x_max = a.x_max.min(b.x_max);
    let y_max = a.y_max.min(b.y_max);

    let intersection = (x_max - x_min).max(0.0) * (y_max - y_min).max(0.0);
    
    let area_a = (a.x_max - a.x_min) * (a.y_max - a.y_min);
    let area_b = (b.x_max - b.x_min) * (b.y_max - b.y_min);
    
    let union = area_a + area_b - intersection;
    
    if union == 0.0 {
        0.0
    } else {
        intersection / union
    }
}

pub fn apply_confidence_threshold(detections: Vec<Detection>, threshold: f32) -> Vec<Detection> {
    detections.into_iter().filter(|d| d.confidence >= threshold).collect()
}

pub fn scale_boxes(boxes: &mut [BoundingBox], src_width: u32, src_height: u32, dst_width: u32, dst_height: u32) {
    let scale_x = dst_width as f32 / src_width as f32;
    let scale_y = dst_height as f32 / src_height as f32;

    for box_mut in boxes.iter_mut() {
        box_mut.x_min *= scale_x;
        box_mut.y_min *= scale_y;
        box_mut.x_max *= scale_x;
        box_mut.y_max *= scale_y;
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_nms() {
        let mut detections = vec![
            Detection { label: 0, confidence: 0.9, bbox: BoundingBox { x_min: 0.0, y_min: 0.0, x_max: 50.0, y_max: 50.0 } },
            Detection { label: 0, confidence: 0.8, bbox: BoundingBox { x_min: 5.0, y_min: 5.0, x_max: 55.0, y_max: 55.0 } },
            Detection { label: 1, confidence: 0.7, bbox: BoundingBox { x_min: 100.0, y_min: 100.0, x_max: 150.0, y_max: 150.0 } },
        ];

        let result = non_max_suppression(&mut detections, 0.5);
        assert_eq!(result.len(), 2);
    }

    #[test]
    fn test_confidence_threshold() {
        let detections = vec![
            Detection { label: 0, confidence: 0.9, bbox: BoundingBox { x_min: 0.0, y_min: 0.0, x_max: 50.0, y_max: 50.0 } },
            Detection { label: 0, confidence: 0.4, bbox: BoundingBox { x_min: 5.0, y_min: 5.0, x_max: 55.0, y_max: 55.0 } },
        ];

        let result = apply_confidence_threshold(detections, 0.5);
        assert_eq!(result.len(), 1);
    }
}
