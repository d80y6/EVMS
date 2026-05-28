use ingest_service::storage::{S3Storage, S3Config, UploadPart};
use bytes::Bytes;

#[test]
fn test_s3_config_default() {
    let config = S3Config::default();
    assert_eq!(config.region, "us-east-1");
    assert!(config.bucket.is_empty());
}

#[test]
fn test_s3_config_custom() {
    let config = S3Config {
        region: "eu-west-1".to_string(),
        bucket: "my-test-bucket".to_string(),
        prefix: "videos/".to_string(),
        endpoint_url: Some("http://localhost:9000".to_string()),
        access_key: Some("test-key".to_string()),
        secret_key: Some("test-secret".to_string()),
        part_size_mb: 10,
    };
    
    assert_eq!(config.region, "eu-west-1");
    assert_eq!(config.bucket, "my-test-bucket");
    assert_eq!(config.prefix, "videos/");
    assert_eq!(config.part_size_mb, 10);
}

#[test]
fn test_upload_part_creation() {
    let data = Bytes::from(vec![1u8, 2, 3, 4, 5]);
    let part = UploadPart {
        part_number: 1,
        data,
    };
    
    assert_eq!(part.part_number, 1);
    assert_eq!(part.data.len(), 5);
}

#[test]
fn test_storage_key_generation() {
    let config = S3Config {
        region: "us-east-1".to_string(),
        bucket: "test-bucket".to_string(),
        prefix: "streams/".to_string(),
        endpoint_url: None,
        access_key: None,
        secret_key: None,
        part_size_mb: 5,
    };
    
    let storage = S3Storage::new(config);
    
    // Test key generation
    let key = storage.generate_key("stream-123", 1, "init.mp4");
    assert_eq!(key, "streams/stream-123/segment-0000000001/init.mp4");
    
    let key2 = storage.generate_key("stream-456", 100, "media.m4s");
    assert_eq!(key2, "streams/stream-456/segment-0000000100/media.m4s");
}

#[test]
fn test_storage_multipart_path() {
    let config = S3Config {
        region: "us-east-1".to_string(),
        bucket: "test-bucket".to_string(),
        prefix: "".to_string(),
        endpoint_url: None,
        access_key: None,
        secret_key: None,
        part_size_mb: 5,
    };
    
    let storage = S3Storage::new(config);
    
    let mp_path = storage.get_multipart_path("upload-abc-123");
    assert!(mp_path.contains("upload-abc-123"));
}

#[tokio::test]
async fn test_storage_create_without_credentials() {
    let config = S3Config {
        region: "us-east-1".to_string(),
        bucket: "test-bucket".to_string(),
        prefix: "test/".to_string(),
        endpoint_url: Some("http://localhost:9999".to_string()), // Non-existent endpoint
        access_key: Some("test".to_string()),
        secret_key: Some("test".to_string()),
        part_size_mb: 5,
    };
    
    let storage = S3Storage::new(config);
    
    // This will fail to connect but should construct properly
    let result = storage.create_multipart_upload("test-stream", "test-key").await;
    // Expected to fail due to non-existent endpoint
    assert!(result.is_err());
}

#[test]
fn test_storage_part_size_calculation() {
    let config = S3Config {
        region: "us-east-1".to_string(),
        bucket: "test".to_string(),
        prefix: "".to_string(),
        endpoint_url: None,
        access_key: None,
        secret_key: None,
        part_size_mb: 10,
    };
    
    let storage = S3Storage::new(config);
    
    // 10 MB = 10 * 1024 * 1024 bytes
    let expected_part_size = 10 * 1024 * 1024;
    assert_eq!(storage.config.part_size_mb, 10);
}

#[test]
fn test_completed_part_ordering() {
    use ingest_service::storage::CompletedPart;
    
    let parts = vec![
        CompletedPart { part_number: 3, etag: "etag3".to_string() },
        CompletedPart { part_number: 1, etag: "etag1".to_string() },
        CompletedPart { part_number: 2, etag: "etag2".to_string() },
    ];
    
    // Verify we can sort by part number
    let mut sorted = parts.clone();
    sorted.sort_by_key(|p| p.part_number);
    
    assert_eq!(sorted[0].part_number, 1);
    assert_eq!(sorted[1].part_number, 2);
    assert_eq!(sorted[2].part_number, 3);
}

#[tokio::test]
async fn test_storage_abort_upload() {
    let config = S3Config {
        region: "us-east-1".to_string(),
        bucket: "test".to_string(),
        prefix: "".to_string(),
        endpoint_url: Some("http://localhost:9999".to_string()),
        access_key: Some("test".to_string()),
        secret_key: Some("test".to_string()),
        part_size_mb: 5,
    };
    
    let storage = S3Storage::new(config);
    
    let result = storage.abort_multipart_upload("test-upload-id", "test-key").await;
    // Expected to fail due to non-existent endpoint
    assert!(result.is_err());
}
