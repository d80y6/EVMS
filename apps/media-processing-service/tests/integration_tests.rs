use media_processing_service::preprocess::{FramePreprocessor, PreprocessConfig, ChannelOrder};
use media_processing_service::postprocess::{Detection, BoundingBox, non_max_suppression, apply_confidence_threshold};

#[test]
fn test_preprocess_bgr() {
    let config = PreprocessConfig {
        channel_order: ChannelOrder::BGR,
        ..Default::default()
    };
    let processor = FramePreprocessor::new(config);
    let frame = vec![100u8; 640 * 480 * 3];
    let result = processor.process(&frame, 640, 480);
    assert!(result.is_ok());
}

#[test]
fn test_nms_same_label() {
    let mut detections = vec![
        Detection { label: 0, confidence: 0.95, bbox: BoundingBox { x_min: 10.0, y_min: 10.0, x_max: 60.0, y_max: 60.0 } },
        Detection { label: 0, confidence: 0.90, bbox: BoundingBox { x_min: 15.0, y_min: 15.0, x_max: 65.0, y_max: 65.0 } },
        Detection { label: 0, confidence: 0.85, bbox: BoundingBox { x_min: 20.0, y_min: 20.0, x_max: 70.0, y_max: 70.0 } },
    ];
    let result = non_max_suppression(&mut detections, 0.5);
    assert_eq!(result.len(), 1);
    assert_eq!(result[0].confidence, 0.95);
}

#[test]
fn test_nms_different_labels() {
    let mut detections = vec![
        Detection { label: 0, confidence: 0.9, bbox: BoundingBox { x_min: 10.0, y_min: 10.0, x_max: 60.0, y_max: 60.0 } },
        Detection { label: 1, confidence: 0.9, bbox: BoundingBox { x_min: 10.0, y_min: 10.0, x_max: 60.0, y_max: 60.0 } },
    ];
    let result = non_max_suppression(&mut detections, 0.5);
    assert_eq!(result.len(), 2);
}

#[test]
fn test_confidence_filter() {
    let detections = vec![
        Detection { label: 0, confidence: 0.8, bbox: BoundingBox { x_min: 0.0, y_min: 0.0, x_max: 50.0, y_max: 50.0 } },
        Detection { label: 0, confidence: 0.6, bbox: BoundingBox { x_min: 10.0, y_min: 10.0, x_max: 60.0, y_max: 60.0 } },
        Detection { label: 0, confidence: 0.4, bbox: BoundingBox { x_min: 20.0, y_min: 20.0, x_max: 70.0, y_max: 70.0 } },
    ];
    let result = apply_confidence_threshold(detections, 0.5);
    assert_eq!(result.len(), 2);
}

#[test]
fn test_empty_detections() {
    let mut detections: Vec<Detection> = vec![];
    let result = non_max_suppression(&mut detections, 0.5);
    assert!(result.is_empty());
}
