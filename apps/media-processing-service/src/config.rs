use serde::{Deserialize, Serialize};
use std::env;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Config {
    pub http_host: String,
    pub http_port: u16,
    pub gst_debug_level: u32,
    pub max_pipelines: usize,
    pub inference_endpoint: Option<String>,
    pub default_input_width: u32,
    pub default_input_height: u32,
    pub default_fps: u32,
}

impl Config {
    pub fn from_env() -> Result<Self, Box<dyn std::error::Error>> {
        Ok(Config {
            http_host: env::var("MEDIA__HTTP_HOST").unwrap_or_else(|_| "0.0.0.0".to_string()),
            http_port: env::var("MEDIA__HTTP_PORT")
                .unwrap_or_else(|_| "3004".to_string())
                .parse()?,
            gst_debug_level: env::var("MEDIA__GST_DEBUG")
                .unwrap_or_else(|_| "2".to_string())
                .parse()?,
            max_pipelines: env::var("MEDIA__MAX_PIPELINES")
                .unwrap_or_else(|_| "10".to_string())
                .parse()?,
            inference_endpoint: env::var("MEDIA__INFERENCE_ENDPOINT").ok(),
            default_input_width: env::var("MEDIA__DEFAULT_WIDTH")
                .unwrap_or_else(|_| "640".to_string())
                .parse()?,
            default_input_height: env::var("MEDIA__DEFAULT_HEIGHT")
                .unwrap_or_else(|_| "480".to_string())
                .parse()?,
            default_fps: env::var("MEDIA__DEFAULT_FPS")
                .unwrap_or_else(|_| "30".to_string())
                .parse()?,
        })
    }
}
