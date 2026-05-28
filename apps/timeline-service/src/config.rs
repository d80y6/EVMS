use serde::{Deserialize, Serialize};
use std::env;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Config {
    pub http_host: String,
    pub http_port: u16,
    pub ntp_servers: Vec<String>,
    pub sync_interval_secs: u64,
    pub max_drift_ms: f64,
    pub alignment_window_ms: u64,
}

impl Config {
    pub fn from_env() -> Result<Self, Box<dyn std::error::Error>> {
        let ntp_servers = env::var("TIMELINE__NTP_SERVERS")
            .unwrap_or_else(|_| "pool.ntp.org,time.google.com".to_string())
            .split(',')
            .map(|s| s.trim().to_string())
            .collect();

        Ok(Config {
            http_host: env::var("TIMELINE__HTTP_HOST").unwrap_or_else(|_| "0.0.0.0".to_string()),
            http_port: env::var("TIMELINE__HTTP_PORT")
                .unwrap_or_else(|_| "3007".to_string())
                .parse()?,
            ntp_servers,
            sync_interval_secs: env::var("TIMELINE__SYNC_INTERVAL")
                .unwrap_or_else(|_| "60".to_string())
                .parse()?,
            max_drift_ms: env::var("TIMELINE__MAX_DRIFT_MS")
                .unwrap_or_else(|_| "100.0".to_string())
                .parse()?,
            alignment_window_ms: env::var("TIMELINE__ALIGNMENT_WINDOW")
                .unwrap_or_else(|_| "500".to_string())
                .parse()?,
        })
    }
}
