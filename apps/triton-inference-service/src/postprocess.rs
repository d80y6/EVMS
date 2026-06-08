use crate::{OutputTensor, TensorData, error::{Error, Result}};

/// Tensor postprocessing operations
pub struct Postprocessor;

impl Postprocessor {
    pub fn new() -> Self {
        Self
    }

    /// Apply softmax activation to output tensor
    pub fn softmax(&self, tensor: &OutputTensor) -> Result<OutputTensor> {
        match &tensor.data {
            TensorData::Fp32(data) => {
                let max_val = data.iter().cloned().fold(f32::NEG_INFINITY, f32::max);
                let exp_sum: f32 = data.iter().map(|&x| (x - max_val).exp()).sum();
                
                let output_data: Vec<f32> = data.iter()
                    .map(|&x| (x - max_val).exp() / exp_sum)
                    .collect();
                
                Ok(OutputTensor {
                    name: tensor.name.clone(),
                    datatype: tensor.datatype.clone(),
                    shape: tensor.shape.clone(),
                    data: TensorData::Fp32(output_data),
                })
            }
            _ => Err(Error::Postprocessing("Only FP32 tensors supported for softmax".to_string())),
        }
    }

    /// Get argmax indices
    pub fn argmax(&self, tensor: &OutputTensor, axis: usize) -> Result<Vec<i64>> {
        match &tensor.data {
            TensorData::Fp32(data) => {
                if tensor.shape.len() <= axis {
                    return Err(Error::Postprocessing("Invalid axis".to_string()));
                }

                let mut indices = Vec::new();
                let dim_size = tensor.shape[axis] as usize;
                let stride: usize = tensor.shape[axis+1..].iter().map(|&d| d as usize).product();
                
                for i in 0..(data.len() / dim_size) {
                    let start = i * dim_size * stride;
                    let mut max_idx = 0;
                    let mut max_val = f32::NEG_INFINITY;
                    
                    for j in 0..dim_size {
                        let idx = start + (j * stride);
                        if idx < data.len() && data[idx] > max_val {
                            max_val = data[idx];
                            max_idx = j;
                        }
                    }
                    indices.push(max_idx as i64);
                }
                
                Ok(indices)
            }
            _ => Err(Error::Postprocessing("Only FP32 tensors supported".to_string())),
        }
    }

    /// Non-Maximum Suppression for object detection
    pub fn nms(&self, boxes: &[f32], scores: &[f32], iou_threshold: f32, max_boxes: usize) -> Result<Vec<usize>> {
        if boxes.len() % 4 != 0 {
            return Err(Error::Postprocessing("Boxes must be multiples of 4".to_string()));
        }

        let num_boxes = boxes.len() / 4;
        if scores.len() != num_boxes {
            return Err(Error::Postprocessing("Scores length must match boxes count".to_string()));
        }

        // Sort by score descending
        let mut indices: Vec<usize> = (0..num_boxes).collect();
        indices.sort_by(|&a, &b| scores[b].partial_cmp(&scores[a]).unwrap_or(std::cmp::Ordering::Equal));

        let mut selected = Vec::new();
        let mut suppressed = vec![false; num_boxes];

        for &idx in &indices {
            if suppressed[idx] || selected.len() >= max_boxes {
                continue;
            }

            selected.push(idx);

            for &other_idx in &indices {
                if other_idx == idx || suppressed[other_idx] {
                    continue;
                }

                let iou = self.calculate_iou(boxes, idx, other_idx);
                if iou > iou_threshold {
                    suppressed[other_idx] = true;
                }
            }
        }

        Ok(selected)
    }

    fn calculate_iou(&self, boxes: &[f32], idx1: usize, idx2: usize) -> f32 {
        let x1_1 = boxes[idx1 * 4];
        let y1_1 = boxes[idx1 * 4 + 1];
        let x2_1 = boxes[idx1 * 4 + 2];
        let y2_1 = boxes[idx1 * 4 + 3];

        let x1_2 = boxes[idx2 * 4];
        let y1_2 = boxes[idx2 * 4 + 1];
        let x2_2 = boxes[idx2 * 4 + 2];
        let y2_2 = boxes[idx2 * 4 + 3];

        let xi1 = x1_1.max(x1_2);
        let yi1 = y1_1.max(y1_2);
        let xi2 = x2_1.min(x2_2);
        let yi2 = y2_1.min(y2_2);

        let inter_w = (xi2 - xi1).max(0.0);
        let inter_h = (yi2 - yi1).max(0.0);
        let inter_area = inter_w * inter_h;

        let area1 = (x2_1 - x1_1) * (y2_1 - y1_1);
        let area2 = (x2_2 - x1_2) * (y2_2 - y1_2);

        let union_area = area1 + area2 - inter_area;
        if union_area == 0.0 {
            0.0
        } else {
            inter_area / union_area
        }
    }

    /// Decode bounding box coordinates from model output
    pub fn decode_boxes(&self, boxes: &OutputTensor, anchors: &[f32]) -> Result<Vec<f32>> {
        match &boxes.data {
            TensorData::Fp32(data) => {
                let mut decoded = Vec::with_capacity(data.len());
                
                for (i, &val) in data.iter().enumerate() {
                    let anchor_idx = i % anchors.len();
                    let decoded_val = val * anchors[anchor_idx];
                    decoded.push(decoded_val);
                }
                
                Ok(decoded)
            }
            _ => Err(Error::Postprocessing("Only FP32 tensors supported".to_string())),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_softmax() {
        let tensor = OutputTensor {
            name: "output".to_string(),
            datatype: TensorDataType::Fp32,
            shape: vec![1, 3],
            data: TensorData::Fp32(vec![1.0, 2.0, 3.0]),
        };

        let postprocessor = Postprocessor::new();
        let result = postprocessor.softmax(&tensor).unwrap();
        
        if let TensorData::Fp32(data) = &result.data {
            // Sum should be approximately 1.0
            let sum: f32 = data.iter().sum();
            assert!((sum - 1.0).abs() < 0.001);
            
            // Values should be in ascending order (since input was)
            assert!(data[0] < data[1]);
            assert!(data[1] < data[2]);
        } else {
            panic!("Expected FP32 data");
        }
    }

    #[test]
    fn test_nms() {
        // Two overlapping boxes with different scores
        let boxes = vec![
            0.0, 0.0, 10.0, 10.0,  // Box 1
            5.0, 5.0, 15.0, 15.0,  // Box 2 (overlaps with Box 1)
        ];
        let scores = vec![0.9, 0.8];

        let postprocessor = Postprocessor::new();
        let selected = postprocessor.nms(&boxes, &scores, 0.5, 10).unwrap();
        
        // Should select the higher scoring box
        assert_eq!(selected.len(), 1);
        assert_eq!(selected[0], 0);
    }
}
