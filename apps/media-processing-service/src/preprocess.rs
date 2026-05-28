use crate::error::{Result, MediaProcessingError};

#[derive(Debug, Clone)]
pub struct PreprocessConfig {
    pub target_width: u32,
    pub target_height: u32,
    pub normalize_mean: [f32; 3],
    pub normalize_std: [f32; 3],
    pub channel_order: ChannelOrder,
}

#[derive(Debug, Clone)]
pub enum ChannelOrder {
    RGB,
    BGR,
    GRAY,
}

impl Default for PreprocessConfig {
    fn default() -> Self {
        PreprocessConfig {
            target_width: 640,
            target_height: 480,
            normalize_mean: [0.485, 0.456, 0.406],
            normalize_std: [0.229, 0.224, 0.225],
            channel_order: ChannelOrder::RGB,
        }
    }
}

pub struct FramePreprocessor {
    config: PreprocessConfig,
}

impl FramePreprocessor {
    pub fn new(config: PreprocessConfig) -> Self {
        FramePreprocessor { config }
    }

    pub fn process(&self, frame: &[u8], width: u32, height: u32) -> Result<Vec<f32>> {
        if width == 0 || height == 0 {
            return Err(MediaProcessingError::Preprocessing("Invalid frame dimensions".to_string()));
        }

        let mut output = Vec::with_capacity((self.config.target_width * self.config.target_height * 3) as usize);
        
        for y in 0..self.config.target_height {
            for x in 0..self.config.target_width {
                let src_x = (x as f32 * (width as f32 / self.config.target_width as f32)) as u32;
                let src_y = (y as f32 * (height as f32 / self.config.target_height as f32)) as u32;
                
                let src_idx = ((src_y * width + src_x) * 3) as usize;
                if src_idx + 2 >= frame.len() {
                    continue;
                }

                let (r, g, b) = match self.config.channel_order {
                    ChannelOrder::RGB => (frame[src_idx], frame[src_idx + 1], frame[src_idx + 2]),
                    ChannelOrder::BGR => (frame[src_idx + 2], frame[src_idx + 1], frame[src_idx]),
                    ChannelOrder::GRAY => {
                        let gray = frame[src_idx];
                        (gray, gray, gray)
                    }
                };

                output.push(((r as f32 / 255.0) - self.config.normalize_mean[0]) / self.config.normalize_std[0]);
                output.push(((g as f32 / 255.0) - self.config.normalize_mean[1]) / self.config.normalize_std[1]);
                output.push(((b as f32 / 255.0) - self.config.normalize_mean[2]) / self.config.normalize_std[2]);
            }
        }

        Ok(output)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_preprocess_rgb() {
        let config = PreprocessConfig::default();
        let processor = FramePreprocessor::new(config);
        let frame = vec![255u8; 640 * 480 * 3];
        let result = processor.process(&frame, 640, 480);
        assert!(result.is_ok());
        assert_eq!(result.unwrap().len(), (640 * 480 * 3) as usize);
    }
}
