use crate::{InputTensor, TensorDataType, TensorData, error::{Error, Result}};

/// Tensor preprocessing operations
pub struct Preprocessor;

impl Preprocessor {
    pub fn new() -> Self {
        Self
    }

    /// Normalize tensor values to [0, 1] or [-1, 1]
    pub fn normalize(&self, tensor: &mut InputTensor, mean: &[f32], std: &[f32]) -> Result<()> {
        match &mut tensor.data {
            TensorData::Fp32(data) => {
                for (i, val) in data.iter_mut().enumerate() {
                    let channel = i % mean.len();
                    *val = (*val - mean[channel]) / std[channel];
                }
            }
            _ => return Err(Error::Preprocessing("Only FP32 tensors can be normalized".to_string())),
        }
        Ok(())
    }

    /// Resize image tensor using nearest neighbor interpolation
    pub fn resize_nearest(&self, tensor: &InputTensor, target_h: i64, target_w: i64) -> Result<InputTensor> {
        if tensor.shape.len() != 4 {
            return Err(Error::Preprocessing("Expected 4D tensor (N,H,W,C)".to_string()));
        }

        let (n, h, w, c) = (tensor.shape[0], tensor.shape[1], tensor.shape[2], tensor.shape[3]);
        
        match &tensor.data {
            TensorData::Fp32(data) => {
                let mut output_data = Vec::with_capacity((n * target_h * target_w * c) as usize);
                
                for ni in 0..n as usize {
                    for ti in 0..target_h as usize {
                        for tj in 0..target_w as usize {
                            // Nearest neighbor mapping
                            let si = (ti as f32 * (h as f32 / target_h as f32)) as usize;
                            let sj = (tj as f32 * (w as f32 / target_w as f32)) as usize;
                            
                            for ci in 0..c as usize {
                                let src_idx = ((ni * h as usize * w as usize * c as usize) 
                                    + (si * w as usize * c as usize) 
                                    + (sj * c as usize) 
                                    + ci) as usize;
                                
                                if src_idx < data.len() {
                                    output_data.push(data[src_idx]);
                                } else {
                                    output_data.push(0.0);
                                }
                            }
                        }
                    }
                }

                Ok(InputTensor {
                    name: tensor.name.clone(),
                    datatype: tensor.datatype.clone(),
                    shape: vec![n, target_h, target_w, c],
                    data: TensorData::Fp32(output_data),
                })
            }
            _ => Err(Error::Preprocessing("Only FP32 tensors supported for resize".to_string())),
        }
    }

    /// Convert HWC layout to CHW layout
    pub fn hwc_to_chw(&self, tensor: &InputTensor) -> Result<InputTensor> {
        if tensor.shape.len() != 4 {
            return Err(Error::Preprocessing("Expected 4D tensor (N,H,W,C)".to_string()));
        }

        let (n, h, w, c) = (tensor.shape[0], tensor.shape[1], tensor.shape[2], tensor.shape[3]);
        
        match &tensor.data {
            TensorData::Fp32(data) => {
                let mut output_data = vec![0.0f32; data.len()];
                
                for ni in 0..n as usize {
                    for ci in 0..c as usize {
                        for hi in 0..h as usize {
                            for wi in 0..w as usize {
                                let src_idx = ((ni * h as usize * w as usize * c as usize) 
                                    + (hi * w as usize * c as usize) 
                                    + (wi * c as usize) 
                                    + ci) as usize;
                                
                                let dst_idx = ((ni * c as usize * h as usize * w as usize) 
                                    + (ci * h as usize * w as usize) 
                                    + (hi * w as usize) 
                                    + wi) as usize;
                                
                                if src_idx < data.len() && dst_idx < output_data.len() {
                                    output_data[dst_idx] = data[src_idx];
                                }
                            }
                        }
                    }
                }

                Ok(InputTensor {
                    name: tensor.name.clone(),
                    datatype: tensor.datatype.clone(),
                    shape: vec![n, c, h, w],
                    data: TensorData::Fp32(output_data),
                })
            }
            _ => Err(Error::Preprocessing("Only FP32 tensors supported".to_string())),
        }
    }

    /// Convert FP32 to FP16
    pub fn to_fp16(&self, tensor: &InputTensor) -> Result<InputTensor> {
        match &tensor.data {
            TensorData::Fp32(data) => {
                let fp16_data: Vec<u16> = data.iter()
                    .map(|&f| half::f16::from_f32(f).to_bits())
                    .collect();
                
                Ok(InputTensor {
                    name: tensor.name.clone(),
                    datatype: TensorDataType::Fp16,
                    shape: tensor.shape.clone(),
                    data: TensorData::Fp16(fp16_data),
                })
            }
            _ => Err(Error::Preprocessing("Can only convert FP32 to FP16".to_string())),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_normalize() {
        let mut tensor = InputTensor {
            name: "input".to_string(),
            datatype: TensorDataType::Fp32,
            shape: vec![1, 3],
            data: TensorData::Fp32(vec![0.0, 0.5, 1.0]),
        };

        let preprocessor = Preprocessor::new();
        let mean = vec![0.0, 0.0, 0.0];
        let std = vec![1.0, 1.0, 1.0];
        
        preprocessor.normalize(&mut tensor, &mean, &std).unwrap();
        
        if let TensorData::Fp32(data) = &tensor.data {
            assert!((data[0] - 0.0).abs() < 0.001);
            assert!((data[1] - 0.5).abs() < 0.001);
            assert!((data[2] - 1.0).abs() < 0.001);
        } else {
            panic!("Expected FP32 data");
        }
    }
}
