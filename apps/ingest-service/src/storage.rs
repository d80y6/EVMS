//! S3 multipart upload storage backend

use aws_config::meta::region::RegionProviderChain;
use aws_sdk_s3::{
    types::{CompletedMultipartUpload, CompletedPart},
    Client,
};
use bytes::Bytes;
use parking_lot::RwLock;
use std::collections::HashMap;
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::sync::mpsc;
use tracing::{debug, error, info, warn};

use crate::error::{Error, Result};
use crate::metrics;
use crate::muxer::Fmp4Segment;

/// S3 upload job
#[derive(Debug, Clone)]
pub struct UploadJob {
    pub key: String,
    pub bucket: String,
    pub upload_id: Option<String>,
    pub parts: Vec<(i32, String)>, // (part_number, etag)
    pub next_part_number: i32,
    pub created_at: Instant,
}

/// S3 uploader with multipart support
#[derive(Debug)]
pub struct S3Uploader {
    client: Option<Client>,
    bucket: String,
    region: String,
    endpoint: Option<String>,
    jobs: Arc<RwLock<HashMap<String, UploadJob>>>,
    segments_rx: Arc<RwLock<Option<mpsc::Receiver<Fmp4Segment>>>>,
    base_path: String,
}

impl S3Uploader {
    /// Create a new S3 uploader
    pub async fn new(
        bucket: String,
        region: String,
        endpoint: Option<String>,
        access_key: Option<String>,
        secret_key: Option<String>,
    ) -> Result<Self> {
        let client = if access_key.is_some() && secret_key.is_some() {
            // Use provided credentials
            let config = aws_config::defaults(aws_config::BehaviorVersion::latest())
                .region(region.clone())
                .load()
                .await;
            
            Some(Client::new(&config))
        } else {
            // Try to create client without explicit credentials (instance role, env vars, etc.)
            match aws_config::defaults(aws_config::BehaviorVersion::latest())
                .region(region.clone())
                .load()
                .await
            {
                config => Some(Client::new(&config)),
            }
        };
        
        Ok(Self {
            client,
            bucket,
            region,
            endpoint,
            jobs: Arc::new(RwLock::new(HashMap::new())),
            segments_rx: Arc::new(RwLock::new(None)),
            base_path: "segments".to_string(),
        })
    }
    
    /// Set the base path for uploads
    pub fn set_base_path(&mut self, path: String) {
        self.base_path = path;
    }
    
    /// Get receiver for segments to upload
    pub fn subscribe(&self) -> Option<mpsc::Receiver<Fmp4Segment>> {
        self.segments_rx.write().take()
    }
    
    /// Run the uploader, consuming segments from muxer
    pub async fn run(&self, _muxer: Arc<crate::muxer::Fmp4Muxer>) -> Result<()> {
        // In production, this would receive from muxer's channel
        // For now, we'll implement the upload logic
        
        loop {
            tokio::time::sleep(Duration::from_secs(1)).await;
            // Process pending uploads
            self.process_pending_uploads().await?;
        }
    }
    
    /// Process pending uploads
    async fn process_pending_uploads(&self) -> Result<()> {
        let jobs = self.jobs.read();
        for (key, job) in jobs.iter() {
            if job.parts.is_empty() {
                continue;
            }
            // Check if upload is complete and should be finalized
            // This is a simplified implementation
        }
        Ok(())
    }
    
    /// Start a multipart upload
    pub async fn start_multipart_upload(&self, key: &str) -> Result<String> {
        let client = self.client.as_ref()
            .ok_or_else(|| Error::Storage("S3 client not configured".into()))?;
        
        let response = client
            .create_multipart_upload()
            .bucket(&self.bucket)
            .key(key)
            .send()
            .await
            .map_err(|e| Error::S3(e.to_string()))?;
        
        let upload_id = response.upload_id()
            .ok_or_else(|| Error::S3("No upload ID returned".into()))?
            .to_string();
        
        let job = UploadJob {
            key: key.to_string(),
            bucket: self.bucket.clone(),
            upload_id: Some(upload_id.clone()),
            parts: Vec::new(),
            next_part_number: 1,
            created_at: Instant::now(),
        };
        
        self.jobs.write().insert(key.to_string(), job);
        
        debug!("Started multipart upload {} for key {}", upload_id, key);
        
        Ok(upload_id)
    }
    
    /// Upload a part
    pub async fn upload_part(&self, key: &str, upload_id: &str, part_number: i32, data: Bytes) -> Result<String> {
        let client = self.client.as_ref()
            .ok_or_else(|| Error::Storage("S3 client not configured".into()))?;
        
        let response = client
            .upload_part()
            .bucket(&self.bucket)
            .key(key)
            .upload_id(upload_id)
            .part_number(part_number)
            .body(data.into())
            .send()
            .await
            .map_err(|e| Error::S3(e.to_string()))?;
        
        let etag = response.etag()
            .ok_or_else(|| Error::S3("No ETag returned".into()))?
            .to_string();
        
        // Update job
        if let Some(job) = self.jobs.write().get_mut(key) {
            job.parts.push((part_number, etag.clone()));
            job.next_part_number = part_number + 1;
        }
        
        debug!("Uploaded part {} for key {}: {}", part_number, key, etag);
        
        Ok(etag)
    }
    
    /// Complete a multipart upload
    pub async fn complete_multipart_upload(&self, key: &str, upload_id: &str) -> Result<()> {
        let client = self.client.as_ref()
            .ok_or_else(|| Error::Storage("S3 client not configured".into()))?;
        
        let job = self.jobs.read()
            .get(key)
            .cloned()
            .ok_or_else(|| Error::Storage(format!("Upload job not found for key {}", key)))?;
        
        let mut parts = Vec::new();
        for (part_number, etag) in &job.parts {
            parts.push(
                CompletedPart::builder()
                    .part_number(*part_number)
                    .etag(etag)
                    .build()
            );
        }
        
        let completed_upload = CompletedMultipartUpload::builder()
            .set_parts(Some(parts))
            .build();
        
        client
            .complete_multipart_upload()
            .bucket(&self.bucket)
            .key(key)
            .upload_id(upload_id)
            .multipart_upload(completed_upload)
            .send()
            .await
            .map_err(|e| Error::S3(e.to_string()))?;
        
        // Clean up job
        self.jobs.write().remove(key);
        
        info!("Completed multipart upload for key {}", key);
        
        Ok(())
    }
    
    /// Abort a multipart upload
    pub async fn abort_multipart_upload(&self, key: &str, upload_id: &str) -> Result<()> {
        let client = self.client.as_ref()
            .ok_or_else(|| Error::Storage("S3 client not configured".into()))?;
        
        client
            .abort_multipart_upload()
            .bucket(&self.bucket)
            .key(key)
            .upload_id(upload_id)
            .send()
            .await
            .map_err(|e| Error::S3(e.to_string()))?;
        
        self.jobs.write().remove(key);
        
        warn!("Aborted multipart upload for key {}", key);
        
        Ok(())
    }
    
    /// Upload a single object (non-multipart)
    pub async fn put_object(&self, key: &str, data: Bytes, content_type: Option<&str>) -> Result<()> {
        let client = self.client.as_ref()
            .ok_or_else(|| Error::Storage("S3 client not configured".into()))?;
        
        let start = Instant::now();
        let size = data.len();
        
        let mut req = client
            .put_object()
            .bucket(&self.bucket)
            .key(key)
            .body(data.into());
        
        if let Some(ct) = content_type {
            req = req.content_type(ct);
        }
        
        req.send()
            .await
            .map_err(|e| Error::S3(e.to_string()))?;
        
        let duration_ms = start.elapsed().as_millis() as u64;
        metrics::record_s3_upload(size, duration_ms, true);
        
        debug!("Uploaded {} bytes to {} in {}ms", size, key, duration_ms);
        
        Ok(())
    }
    
    /// Generate S3 key for a segment
    pub fn generate_segment_key(&self, stream_id: &str, segment_number: u32) -> String {
        format!("{}/{}/segment_{:06}.m4s", self.base_path, stream_id, segment_number)
    }
    
    /// Generate S3 key for init segment
    pub fn generate_init_key(&self, stream_id: &str) -> String {
        format!("{}/{}/init.mp4", self.base_path, stream_id)
    }
    
    /// Upload a segment
    pub async fn upload_segment(&self, stream_id: &str, segment: Fmp4Segment) -> Result<()> {
        let key = self.generate_segment_key(stream_id, segment.sequence_number);
        
        // Combine init and media segments if init is present
        let data = if !segment.init_segment.is_empty() {
            let mut combined = Vec::with_capacity(segment.init_segment.len() + segment.media_segment.len());
            combined.extend_from_slice(&segment.init_segment);
            combined.extend_from_slice(&segment.media_segment);
            Bytes::from(combined)
        } else {
            segment.media_segment
        };
        
        self.put_object(&key, data, Some("video/mp4")).await?;
        
        Ok(())
    }
    
    /// Delete an object
    pub async fn delete_object(&self, key: &str) -> Result<()> {
        let client = self.client.as_ref()
            .ok_or_else(|| Error::Storage("S3 client not configured".into()))?;
        
        client
            .delete_object()
            .bucket(&self.bucket)
            .key(key)
            .send()
            .await
            .map_err(|e| Error::S3(e.to_string()))?;
        
        debug!("Deleted object {}", key);
        
        Ok(())
    }
    
    /// List objects with prefix
    pub async fn list_objects(&self, prefix: &str) -> Result<Vec<String>> {
        let client = self.client.as_ref()
            .ok_or_else(|| Error::Storage("S3 client not configured".into()))?;
        
        let response = client
            .list_objects_v2()
            .bucket(&self.bucket)
            .prefix(prefix)
            .send()
            .await
            .map_err(|e| Error::S3(e.to_string()))?;
        
        let keys: Vec<String> = response.contents()
            .iter()
            .filter_map(|obj| obj.key().map(String::from))
            .collect();
        
        Ok(keys)
    }
}
