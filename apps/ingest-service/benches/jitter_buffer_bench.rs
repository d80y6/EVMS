use criterion::{black_box, criterion_group, criterion_main, Criterion, BenchmarkId};
use ingest_service::rtp::{JitterBuffer, JitterBufferConfig};
use std::time::Duration;

fn criterion_benchmark(c: &mut Criterion) {
    // Benchmark: Insert packets in order
    c.bench_function("jitter_buffer_insert_ordered_100", |b| {
        b.iter(|| {
            let config = JitterBufferConfig {
                max_size: 1000,
                max_delay_ms: 500,
            };
            let mut buffer = JitterBuffer::new(config);
            for i in 0..100 {
                buffer.insert(i, vec![i as u8; 100]);
            }
            black_box(buffer);
        })
    });

    // Benchmark: Insert packets out of order
    c.bench_function("jitter_buffer_insert_out_of_order_100", |b| {
        b.iter(|| {
            let config = JitterBufferConfig {
                max_size: 1000,
                max_delay_ms: 500,
            };
            let mut buffer = JitterBuffer::new(config);
            
            // Create shuffled sequence
            let mut seqs: Vec<u16> = (0..100).collect();
            seqs.reverse(); // Worst case: reverse order
            
            for seq in seqs {
                buffer.insert(seq, vec![seq as u8; 100]);
            }
            black_box(buffer);
        })
    });

    // Benchmark: Pop available packets
    c.bench_function("jitter_buffer_pop_available_100", |b| {
        b.iter(|| {
            let config = JitterBufferConfig {
                max_size: 1000,
                max_delay_ms: 500,
            };
            let mut buffer = JitterBuffer::new(config);
            
            // Pre-fill buffer
            for i in 0..100 {
                buffer.insert(i, vec![i as u8; 100]);
            }
            
            // Pop all
            while let Some(_) = buffer.pop_available() {}
            black_box(());
        })
    });

    // Benchmark: Loss detection with gaps
    c.bench_function("jitter_buffer_loss_detection", |b| {
        b.iter(|| {
            let config = JitterBufferConfig {
                max_size: 1000,
                max_delay_ms: 500,
            };
            let mut buffer = JitterBuffer::new(config);
            
            // Insert with gaps: 0, 1, 3, 4, 6, 7, 9, 10...
            for i in 0..100 {
                if i % 5 != 2 { // Skip every 5th packet (20% loss)
                    buffer.insert(i, vec![i as u8; 100]);
                }
            }
            black_box(buffer.get_stats());
        })
    });

    // Benchmark: Large buffer stress test
    c.bench_function("jitter_buffer_stress_1000", |b| {
        b.iter(|| {
            let config = JitterBufferConfig {
                max_size: 2000,
                max_delay_ms: 1000,
            };
            let mut buffer = JitterBuffer::new(config);
            
            for i in 0..1000 {
                let seq = (i * 7) % 1000; // Pseudo-random order
                buffer.insert(seq as u16, vec![seq as u8; 1400]); // MTU-sized
            }
            
            // Pop what's available
            while let Some(_) = buffer.pop_available() {}
            black_box(buffer.get_stats());
        })
    });
}

fn bench_with_sizes(c: &mut Criterion) {
    let mut group = c.benchmark_group("jitter_buffer_sizes");
    
    for size in [10, 50, 100, 500, 1000].iter() {
        group.bench_with_input(BenchmarkId::from_parameter(size), size, |b, &size| {
            b.iter(|| {
                let config = JitterBufferConfig {
                    max_size: size * 2,
                    max_delay_ms: 500,
                };
                let mut buffer = JitterBuffer::new(config);
                
                for i in 0..size {
                    buffer.insert(i as u16, vec![i as u8; 100]);
                }
                
                while let Some(_) = buffer.pop_available() {}
                black_box(buffer.get_stats());
            })
        });
    }
    group.finish();
}

criterion_group!(benches, criterion_benchmark, bench_with_sizes);
criterion_main!(benches);
