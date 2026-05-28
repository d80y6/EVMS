use crate::error::{Error, Result};

pub struct Quantizer {
    enabled: bool,
}

impl Quantizer {
    pub fn new(enabled: bool) -> Self {
        Self { enabled }
    }

    pub fn quantize_fp16(&self, vectors: &[Vec<f32>]) -> Result<Vec<Vec<u16>>> {
        if !self.enabled {
            return Err(Error::Quantization("Quantization disabled".to_string()));
        }
        
        let mut result = Vec::new();
        for vec in vectors {
            let quantized: Vec<u16> = vec.iter()
                .map(|&f| half::f16::from_f32(f).to_bits())
                .collect();
            result.push(quantized);
        }
        Ok(result)
    }

    pub fn dequantize_fp16(&self, vectors: &[Vec<u16>]) -> Result<Vec<Vec<f32>>> {
        if !self.enabled {
            return Err(Error::Quantization("Quantization disabled".to_string()));
        }
        
        let mut result = Vec::new();
        for vec in vectors {
            let dequantized: Vec<f32> = vec.iter()
                .map(|&b| half::f16::from_bits(b).to_f32())
                .collect();
            result.push(dequantized);
        }
        Ok(result)
    }
}
