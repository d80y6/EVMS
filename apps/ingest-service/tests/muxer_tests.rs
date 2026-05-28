use ingest_service::muxer::{Fmp4Muxer, MuxerConfig, Segment};
use ingest_service::rtp::RtpPacket;
use std::time::Duration;

#[test]
fn test_muxer_create() {
    let config = MuxerConfig {
        segment_duration_ms: 2000,
        video_timescale: 90000,
        audio_timescale: 48000,
    };
    
    let muxer = Fmp4Muxer::new(config);
    assert!(muxer.get_current_segment().is_none());
}

#[test]
fn test_muxer_init_segment() {
    let config = MuxerConfig {
        segment_duration_ms: 2000,
        video_timescale: 90000,
        audio_timescale: 48000,
    };
    
    let mut muxer = Fmp4Muxer::new(config);
    muxer.init_segment(1, 90000, 48000);
    
    let segment = muxer.get_current_segment().unwrap();
    assert_eq!(segment.seq, 1);
    assert!(segment.init_data.is_some());
}

#[test]
fn test_muxer_add_video_sample() {
    let config = MuxerConfig {
        segment_duration_ms: 2000,
        video_timescale: 90000,
        audio_timescale: 48000,
    };
    
    let mut muxer = Fmp4Muxer::new(config);
    muxer.init_segment(1, 90000, 48000);
    
    // Add video sample
    let sample_data = vec![0x00, 0x00, 0x00, 0x01, 0x67]; // H.264 NAL start
    muxer.add_video_sample(sample_data.clone(), 0, Duration::from_millis(33), true);
    
    let segment = muxer.get_current_segment().unwrap();
    assert_eq!(segment.video_samples, 1);
    assert_eq!(segment.total_duration_ms, 33);
}

#[test]
fn test_muxer_add_audio_sample() {
    let config = MuxerConfig {
        segment_duration_ms: 2000,
        video_timescale: 90000,
        audio_timescale: 48000,
    };
    
    let mut muxer = Fmp4Muxer::new(config);
    muxer.init_segment(1, 90000, 48000);
    
    // Add audio sample
    let sample_data = vec![0xFF, 0xF1, 0x0C, 0x00]; // AAC frame
    muxer.add_audio_sample(sample_data.clone(), 0, Duration::from_millis(23));
    
    let segment = muxer.get_current_segment().unwrap();
    assert_eq!(segment.audio_samples, 1);
}

#[test]
fn test_muxer_segment_flush() {
    let config = MuxerConfig {
        segment_duration_ms: 100, // Short for testing
        video_timescale: 90000,
        audio_timescale: 48000,
    };
    
    let mut muxer = Fmp4Muxer::new(config);
    muxer.init_segment(1, 90000, 48000);
    
    // Add samples until segment should flush
    for i in 0..10 {
        let sample_data = vec![i as u8; 100];
        muxer.add_video_sample(sample_data, i * 900, Duration::from_millis(10), i == 0);
    }
    
    // Check that we have media data
    let segment = muxer.get_current_segment().unwrap();
    assert!(segment.video_samples > 0);
}

#[test]
fn test_muxer_generate_moof() {
    let config = MuxerConfig {
        segment_duration_ms: 2000,
        video_timescale: 90000,
        audio_timescale: 48000,
    };
    
    let mut muxer = Fmp4Muxer::new(config);
    muxer.init_segment(1, 90000, 48000);
    
    // Add a video sample
    let sample_data = vec![0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00, 0x1E];
    muxer.add_video_sample(sample_data, 0, Duration::from_millis(33), true);
    
    // Generate MOOF
    let moof = muxer.generate_moof(false).unwrap();
    assert!(!moof.is_empty());
    
    // MOOF should start with box size and type 'moof'
    assert_eq!(moof[4..8], [b'm', b'o', b'o', b'f']);
}

#[test]
fn test_muxer_generate_mdat() {
    let config = MuxerConfig {
        segment_duration_ms: 2000,
        video_timescale: 90000,
        audio_timescale: 48000,
    };
    
    let mut muxer = Fmp4Muxer::new(config);
    muxer.init_segment(1, 90000, 48000);
    
    // Add samples
    let sample_data = vec![0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00, 0x1E, 0xAB, 0xCD];
    muxer.add_video_sample(sample_data.clone(), 0, Duration::from_millis(33), true);
    
    // Generate MDAT
    let mdat = muxer.generate_mdat().unwrap();
    assert!(!mdat.is_empty());
    
    // MDAT should start with box size and type 'mdat'
    assert_eq!(mdat[4..8], [b'm', b'd', b'a', b't']);
    
    // MDAT should contain our sample data
    assert!(mdat.windows(sample_data.len()).any(|w| w == sample_data.as_slice()));
}

#[test]
fn test_muxer_generate_init() {
    let config = MuxerConfig {
        segment_duration_ms: 2000,
        video_timescale: 90000,
        audio_timescale: 48000,
    };
    
    let mut muxer = Fmp4Muxer::new(config);
    
    // Generate init segment
    let init = muxer.generate_init(1, 90000, 48000).unwrap();
    assert!(!init.is_empty());
    
    // Should contain ftyp box
    assert_eq!(init[4..8], [b'f', b't', b'y', b'p']);
    
    // Should contain moov box
    assert!(init.windows(4).any(|w| w == [b'm', b'o', b'o', b'v']));
}

#[test]
fn test_muxer_sample_duration_tracking() {
    let config = MuxerConfig {
        segment_duration_ms: 100,
        video_timescale: 90000,
        audio_timescale: 48000,
    };
    
    let mut muxer = Fmp4Muxer::new(config);
    muxer.init_segment(1, 90000, 48000);
    
    // Add samples with specific durations
    for i in 0..5 {
        let sample_data = vec![i as u8; 50];
        muxer.add_video_sample(sample_data, i * 900, Duration::from_millis(20), i == 0);
    }
    
    let segment = muxer.get_current_segment().unwrap();
    assert_eq!(segment.video_samples, 5);
    assert_eq!(segment.total_duration_ms, 100); // 5 * 20ms
}

#[tokio::test]
async fn test_muxer_concurrent_segments() {
    let config = MuxerConfig {
        segment_duration_ms: 50,
        video_timescale: 90000,
        audio_timescale: 48000,
    };
    
    let mut muxer = Fmp4Muxer::new(config);
    muxer.init_segment(1, 90000, 48000);
    
    // Rapidly add samples
    for i in 0..20 {
        let sample_data = vec![i as u8; 100];
        muxer.add_video_sample(sample_data, i * 450, Duration::from_millis(5), i == 0);
        tokio::time::sleep(Duration::from_millis(1)).await;
    }
    
    let segment = muxer.get_current_segment().unwrap();
    assert!(segment.video_samples > 0);
}
