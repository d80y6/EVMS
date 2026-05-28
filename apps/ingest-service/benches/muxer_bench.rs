use criterion::{black_box, criterion_group, criterion_main, Criterion};
use ingest_service::muxer::{Fmp4Muxer, MuxerConfig};
use std::time::Duration;

fn criterion_benchmark(c: &mut Criterion) {
    // Benchmark: Muxer initialization
    c.bench_function("muxer_init_segment", |b| {
        b.iter(|| {
            let config = MuxerConfig {
                segment_duration_ms: 2000,
                video_timescale: 90000,
                audio_timescale: 48000,
            };
            let mut muxer = Fmp4Muxer::new(config);
            muxer.init_segment(1, 90000, 48000);
            black_box(muxer);
        })
    });

    // Benchmark: Add video samples
    c.bench_function("muxer_add_video_samples_30fps_1sec", |b| {
        b.iter(|| {
            let config = MuxerConfig {
                segment_duration_ms: 2000,
                video_timescale: 90000,
                audio_timescale: 48000,
            };
            let mut muxer = Fmp4Muxer::new(config);
            muxer.init_segment(1, 90000, 48000);
            
            // 30 frames at 33ms each
            for i in 0..30 {
                let sample_data = vec![i as u8; 5000]; // ~5KB per frame
                muxer.add_video_sample(sample_data, i * 3000, Duration::from_millis(33), i == 0);
            }
            black_box(muxer);
        })
    });

    // Benchmark: Add audio samples
    c.bench_function("muxer_add_audio_samples_48khz_1sec", |b| {
        b.iter(|| {
            let config = MuxerConfig {
                segment_duration_ms: 2000,
                video_timescale: 90000,
                audio_timescale: 48000,
            };
            let mut muxer = Fmp4Muxer::new(config);
            muxer.init_segment(1, 90000, 48000);
            
            // ~47 AAC frames per second (1024 samples each at 48kHz)
            for i in 0..47 {
                let sample_data = vec![i as u8; 200]; // ~200 bytes per AAC frame
                muxer.add_audio_sample(sample_data, i * 1024, Duration::from_millis(21));
            }
            black_box(muxer);
        })
    });

    // Benchmark: Generate MOOF box
    c.bench_function("muxer_generate_moof", |b| {
        b.iter(|| {
            let config = MuxerConfig {
                segment_duration_ms: 2000,
                video_timescale: 90000,
                audio_timescale: 48000,
            };
            let mut muxer = Fmp4Muxer::new(config);
            muxer.init_segment(1, 90000, 48000);
            
            // Add some samples first
            for i in 0..10 {
                let sample_data = vec![i as u8; 1000];
                muxer.add_video_sample(sample_data, i * 3000, Duration::from_millis(33), i == 0);
            }
            
            let moof = muxer.generate_moof(false).unwrap();
            black_box(moof);
        })
    });

    // Benchmark: Generate MDAT box
    c.bench_function("muxer_generate_mdat", |b| {
        b.iter(|| {
            let config = MuxerConfig {
                segment_duration_ms: 2000,
                video_timescale: 90000,
                audio_timescale: 48000,
            };
            let mut muxer = Fmp4Muxer::new(config);
            muxer.init_segment(1, 90000, 48000);
            
            // Add samples with larger payloads
            for i in 0..10 {
                let sample_data = vec![i as u8; 10000]; // 10KB per sample
                muxer.add_video_sample(sample_data, i * 3000, Duration::from_millis(33), i == 0);
            }
            
            let mdat = muxer.generate_mdat().unwrap();
            black_box(mdat.len());
        })
    });

    // Benchmark: Full segment generation (init + moof + mdat)
    c.bench_function("muxer_full_segment_1sec", |b| {
        b.iter(|| {
            let config = MuxerConfig {
                segment_duration_ms: 2000,
                video_timescale: 90000,
                audio_timescale: 48000,
            };
            let mut muxer = Fmp4Muxer::new(config);
            muxer.init_segment(1, 90000, 48000);
            
            // 1 second of video at 30fps
            for i in 0..30 {
                let sample_data = vec![i as u8; 5000];
                muxer.add_video_sample(sample_data, i * 3000, Duration::from_millis(33), i == 0);
            }
            
            // Generate boxes
            let init = muxer.generate_init(1, 90000, 48000).unwrap();
            let moof = muxer.generate_moof(false).unwrap();
            let mdat = muxer.generate_mdat().unwrap();
            
            black_box((init.len(), moof.len(), mdat.len()));
        })
    });

    // Benchmark: Init segment generation
    c.bench_function("muxer_generate_init", |b| {
        b.iter(|| {
            let config = MuxerConfig {
                segment_duration_ms: 2000,
                video_timescale: 90000,
                audio_timescale: 48000,
            };
            let muxer = Fmp4Muxer::new(config);
            
            let init = muxer.generate_init(1, 90000, 48000).unwrap();
            black_box(init);
        })
    });

    // Benchmark: Concurrent AV muxing
    c.bench_function("muxer_concurrent_av_1sec", |b| {
        b.iter(|| {
            let config = MuxerConfig {
                segment_duration_ms: 2000,
                video_timescale: 90000,
                audio_timescale: 48000,
            };
            let mut muxer = Fmp4Muxer::new(config);
            muxer.init_segment(1, 90000, 48000);
            
            // Interleave video and audio
            for i in 0..30 {
                // Video frame
                let video_data = vec![i as u8; 5000];
                muxer.add_video_sample(video_data, i * 3000, Duration::from_millis(33), i == 0);
                
                // Audio frames (roughly 1.5 per video frame)
                for j in 0..2 {
                    let audio_idx = i * 2 + j;
                    let audio_data = vec![audio_idx as u8; 200];
                    muxer.add_audio_sample(audio_data, audio_idx * 1024, Duration::from_millis(21));
                }
            }
            black_box(muxer);
        })
    });
}

criterion_group!(benches, criterion_benchmark);
criterion_main!(benches);
