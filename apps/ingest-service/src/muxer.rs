//! fMP4 (Fragmented MP4) muxer

use bytes::{BufMut, Bytes, BytesMut};
use parking_lot::RwLock;
use std::collections::HashMap;
use std::path::Path;
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::fs::File;
use tokio::io::AsyncWriteExt;
use tracing::{debug, error, info};

use crate::error::{Error, Result};
use crate::metrics;
use crate::rtp::{JitterBuffer, RtpPacket};
use crate::storage::S3Uploader;

/// fMP4 segment data
#[derive(Debug, Clone)]
pub struct Fmp4Segment {
    pub sequence_number: u32,
    pub duration_ms: u64,
    pub init_segment: Bytes,
    pub media_segment: Bytes,
    pub created_at: Instant,
}

/// fMP4 Muxer state
#[derive(Debug)]
struct MuxerState {
    sequence_number: u32,
    current_segment_start: Option<Instant>,
    current_segment_packets: Vec<RtpPacket>,
    init_segment_generated: bool,
    track_id: u32,
    timescale: u32,
    last_timestamp: Option<u32>,
    base_timestamp: Option<u64>,
}

impl Default for MuxerState {
    fn default() -> Self {
        Self {
            sequence_number: 1,
            current_segment_start: None,
            current_segment_packets: Vec::with_capacity(1000),
            init_segment_generated: false,
            track_id: 1,
            timescale: 90000, // Common for H.264
            last_timestamp: None,
            base_timestamp: None,
        }
    }
}

/// Fragmented MP4 Muxer
#[derive(Debug)]
pub struct Fmp4Muxer {
    segment_duration: Duration,
    init_segment_path: Option<String>,
    state: Arc<RwLock<MuxerState>>,
    segments_tx: tokio::sync::mpsc::Sender<Fmp4Segment>,
    segments_rx: Arc<RwLock<Option<tokio::sync::mpsc::Receiver<Fmp4Segment>>>>,
}

impl Fmp4Muxer {
    /// Create a new fMP4 muxer
    pub fn new(segment_duration_ms: u64, init_segment_path: Option<&str>) -> Self {
        let (segments_tx, segments_rx) = tokio::sync::mpsc::channel(100);
        
        Self {
            segment_duration: Duration::from_millis(segment_duration_ms),
            init_segment_path: init_segment_path.map(String::from),
            state: Arc::new(RwLock::new(MuxerState::default())),
            segments_tx,
            segments_rx: Arc::new(RwLock::new(Some(segments_rx))),
        }
    }
    
    /// Get receiver for muxed segments
    pub fn subscribe(&self) -> Option<tokio::sync::mpsc::Receiver<Fmp4Segment>> {
        self.segments_rx.write().take()
    }
    
    /// Run the muxer, consuming packets from jitter buffer
    pub async fn run(&self, jitter_buffer: Arc<JitterBuffer>, storage: Arc<S3Uploader>) -> Result<()> {
        let mut interval = tokio::time::interval(self.segment_duration);
        interval.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Delay);
        
        loop {
            tokio::select! {
                _ = interval.tick() => {
                    if let Err(e) = self.flush_segment(jitter_buffer.clone(), storage.clone()).await {
                        error!("Failed to flush segment: {}", e);
                    }
                }
            }
        }
    }
    
    /// Flush current segment
    async fn flush_segment(&self, _jitter_buffer: Arc<JitterBuffer>, storage: Arc<S3Uploader>) -> Result<()> {
        let mut state = self.state.write();
        
        if state.current_segment_packets.is_empty() {
            return Ok(());
        }
        
        let packets = std::mem::take(&mut.state.current_segment_packets);
        let segment_number = state.sequence_number;
        state.sequence_number += 1;
        
        let start_time = state.current_segment_start.unwrap_or_else(Instant::now);
        let duration_ms = start_time.elapsed().as_millis() as u64;
        
        // Generate init segment if needed
        let init_segment = if !state.init_segment_generated {
            let init = self.generate_init_segment()?;
            state.init_segment_generated = true;
            
            // Save init segment to file if path configured
            if let Some(ref path) = self.init_segment_path {
                let mut file = File::create(path).await?;
                file.write_all(&init).await?;
                debug!("Saved init segment to {}", path);
            }
            
            init
        } else {
            Bytes::new()
        };
        
        // Generate media segment
        let media_segment = self.generate_media_segment(&packets, segment_number)?;
        
        let segment = Fmp4Segment {
            sequence_number: segment_number,
            duration_ms,
            init_segment,
            media_segment,
            created_at: Instant::now(),
        };
        
        metrics::record_segment_muxed(duration_ms, segment.media_segment.len());
        
        info!(
            "Muxed segment {} with {} packets, {} bytes, {}ms",
            segment_number,
            packets.len(),
            segment.media_segment.len(),
            duration_ms
        );
        
        // Send to storage
        self.segments_tx.send(segment).await
            .map_err(|e| Error::Muxer(format!("Failed to send segment: {}", e)))?;
        
        // Reset state for next segment
        state.current_segment_start = Some(Instant::now());
        state.current_segment_packets.clear();
        
        Ok(())
    }
    
    /// Generate MP4 initialization segment
    fn generate_init_segment(&self) -> Result<Bytes> {
        let mut buf = BytesMut::with_capacity(1024);
        
        // ftyp box
        let ftyp = self.generate_ftyp();
        buf.put_slice(&ftyp);
        
        // moov box
        let moov = self.generate_moov()?;
        buf.put_slice(&moov);
        
        Ok(buf.freeze())
    }
    
    /// Generate ftyp box
    fn generate_ftyp(&self) -> Bytes {
        let mut buf = BytesMut::new();
        
        // Brand: iso6, compatible brands: iso6, mp41
        let data = b"isomiso6mp41";
        
        buf.put_u32(8 + data.len() as u32); // size
        buf.put_slice(b"ftyp");
        buf.put_slice(b"isom"); // major brand
        buf.put_u32(0);         // minor version
        buf.put_slice(b"iso6"); // compatible brand
        buf.put_slice(b"mp41"); // compatible brand
        
        buf.freeze()
    }
    
    /// Generate moov box
    fn generate_moov(&self) -> Result<Bytes> {
        let mut buf = BytesMut::with_capacity(512);
        
        // mvhd - movie header
        let mvhd = self.generate_mvhd();
        buf.put_slice(&mvhd);
        
        // trak - track
        let trak = self.generate_trak()?;
        buf.put_slice(&trak);
        
        // Wrap in moov
        let mut moov = BytesMut::new();
        moov.put_u32(8 + buf.len() as u32);
        moov.put_slice(b"moov");
        moov.put_slice(&buf);
        
        Ok(moov.freeze())
    }
    
    /// Generate mvhd box
    fn generate_mvhd(&self) -> Bytes {
        let mut buf = BytesMut::with_capacity(108);
        
        buf.put_u32(108); // size
        buf.put_slice(b"mvhd");
        buf.put_u32(0);   // version and flags
        buf.put_u32(0);   // creation time
        buf.put_u32(0);   // modification time
        buf.put_u32(self.timescale); // timescale
        buf.put_u32(0);   // duration (unknown at init)
        buf.put_u32(0x00010000); // rate
        buf.put_u16(0x0100);     // volume
        buf.put_u16(0);          // reserved
        buf.put_u32(0);          // reserved
        buf.put_u32(0);          // reserved
        // Matrix
        buf.put_u32(0x00010000);
        buf.put_u32(0);
        buf.put_u32(0);
        buf.put_u32(0);
        buf.put_u32(0x00010000);
        buf.put_u32(0);
        buf.put_u32(0);
        buf.put_u32(0);
        buf.put_u32(0x40000000);
        buf.put_u32(0); // pre-defined
        buf.put_u32(0); // pre-defined
        buf.put_u32(0); // pre-defined
        buf.put_u32(0); // pre-defined
        buf.put_u32(0); // pre-defined
        buf.put_u32(0); // pre-defined
        buf.put_u32(state.track_id + 1); // next track ID
        
        buf.freeze()
    }
    
    /// Generate trak box
    fn generate_trak(&self) -> Result<Bytes> {
        let mut buf = BytesMut::with_capacity(256);
        
        // tkhd - track header
        let tkhd = self.generate_tkhd();
        buf.put_slice(&tkhd);
        
        // mdia - media
        let mdia = self.generate_mdia()?;
        buf.put_slice(&mdia);
        
        // Wrap in trak
        let mut trak = BytesMut::new();
        trak.put_u32(8 + buf.len() as u32);
        trak.put_slice(b"trak");
        trak.put_slice(&buf);
        
        Ok(trak.freeze())
    }
    
    /// Generate tkhd box
    fn generate_tkhd(&self) -> Bytes {
        let mut buf = BytesMut::with_capacity(92);
        
        buf.put_u32(92); // size
        buf.put_slice(b"tkhd");
        buf.put_u32(0);  // version and flags (track enabled)
        buf.put_u32(0);  // creation time
        buf.put_u32(0);  // modification time
        buf.put_u32(state.track_id); // track ID
        buf.put_u32(0);  // reserved
        buf.put_u32(0);  // duration (unknown)
        buf.put_u32(0);  // reserved
        buf.put_u32(0);  // reserved
        buf.put_u16(0);  // layer
        buf.put_u16(0);  // alternate group
        buf.put_u16(0x0100); // volume
        buf.put_u16(0);  // reserved
        // Matrix
        buf.put_u32(0x00010000);
        buf.put_u32(0);
        buf.put_u32(0);
        buf.put_u32(0);
        buf.put_u32(0x00010000);
        buf.put_u32(0);
        buf.put_u32(0);
        buf.put_u32(0);
        buf.put_u32(0x40000000);
        buf.put_u32(0); // width
        buf.put_u32(0); // height
        
        buf.freeze()
    }
    
    /// Generate mdia box
    fn generate_mdia(&self) -> Result<Bytes> {
        let mut buf = BytesMut::with_capacity(128);
        
        // mdhd - media header
        let mdhd = self.generate_mdhd();
        buf.put_slice(&mdhd);
        
        // hdlr - handler reference
        let hdlr = self.generate_hdlr();
        buf.put_slice(&hdlr);
        
        // minf - media information
        let minf = self.generate_minf()?;
        buf.put_slice(&minf);
        
        // Wrap in mdia
        let mut mdia = BytesMut::new();
        mdia.put_u32(8 + buf.len() as u32);
        mdia.put_slice(b"mdia");
        mdia.put_slice(&buf);
        
        Ok(mdia.freeze())
    }
    
    fn generate_mdhd(&self) -> Bytes {
        let mut buf = BytesMut::with_capacity(32);
        
        buf.put_u32(32); // size
        buf.put_slice(b"mdhd");
        buf.put_u32(0);  // version and flags
        buf.put_u32(0);  // creation time
        buf.put_u32(0);  // modification time
        buf.put_u32(self.timescale); // timescale
        buf.put_u32(0);  // duration
        buf.put_u16(0x55C4); // language (undetermined)
        buf.put_u16(0);  // quality
        
        buf.freeze()
    }
    
    fn generate_hdlr(&self) -> Bytes {
        let mut buf = BytesMut::with_capacity(52);
        
        buf.put_u32(52); // size
        buf.put_slice(b"hdlr");
        buf.put_u32(0);  // version and flags
        buf.put_u32(0);  // pre-defined
        buf.put_slice(b"vide"); // handler type (video)
        buf.put_u32(0);  // reserved
        buf.put_u32(0);  // reserved
        buf.put_u32(0);  // reserved
        buf.put_slice(b"VideoHandler\0"); // name
        
        buf.freeze()
    }
    
    fn generate_minf(&self) -> Result<Bytes> {
        let mut buf = BytesMut::with_capacity(64);
        
        // vmhd - video media header
        let vmhd = self.generate_vmhd();
        buf.put_slice(&vmhd);
        
        // dinf - data information
        let dinf = self.generate_dinf();
        buf.put_slice(&dinf);
        
        // stbl - sample table
        let stbl = self.generate_stbl()?;
        buf.put_slice(&stbl);
        
        // Wrap in minf
        let mut minf = BytesMut::new();
        minf.put_u32(8 + buf.len() as u32);
        minf.put_slice(b"minf");
        minf.put_slice(&buf);
        
        Ok(minf.freeze())
    }
    
    fn generate_vmhd(&self) -> Bytes {
        let mut buf = BytesMut::with_capacity(20);
        
        buf.put_u32(20); // size
        buf.put_slice(b"vmhd");
        buf.put_u32(0);  // version and flags
        buf.put_u16(0);  // graphics mode
        buf.put_u16(0);  // opcolor R
        buf.put_u16(0);  // opcolor G
        buf.put_u16(0);  // opcolor B
        
        buf.freeze()
    }
    
    fn generate_dinf(&self) -> Bytes {
        let mut buf = BytesMut::with_capacity(36);
        
        // dref - data reference
        let mut dref_data = BytesMut::new();
        dref_data.put_u32(0); // version and flags
        dref_data.put_u32(1); // entry count
        
        // url box
        let url = b"\x00\x00\x00\x0curl\x00\x00\x00\x01";
        dref_data.put_slice(url);
        
        buf.put_u32(8 + dref_data.len() as u32);
        buf.put_slice(b"dinf");
        buf.put_u32(8 + dref_data.len() as u32);
        buf.put_slice(b"dref");
        buf.put_slice(&dref_data);
        
        buf.freeze()
    }
    
    fn generate_stbl(&self) -> Result<Bytes> {
        let mut buf = BytesMut::with_capacity(128);
        
        // stsd - sample description
        let stsd = self.generate_stsd()?;
        buf.put_slice(&stsd);
        
        // stts - time-to-sample
        let stts = b"\x00\x00\x00\x10stts\x00\x00\x00\x00\x00\x00\x00\x00";
        buf.put_slice(stts);
        
        // stss - sync sample
        let stss = b"\x00\x00\x00\x10stss\x00\x00\x00\x00\x00\x00\x00\x00";
        buf.put_slice(stss);
        
        // stsc - sample-to-chunk
        let stsc = b"\x00\x00\x00\x10stsc\x00\x00\x00\x00\x00\x00\x00\x00";
        buf.put_slice(stsc);
        
        // stsz - sample size
        let stsz = b"\x00\x00\x00\x10stsz\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00";
        buf.put_slice(stsz);
        
        // stco - chunk offset
        let stco = b"\x00\x00\x00\x10stco\x00\x00\x00\x00\x00\x00\x00\x00";
        buf.put_slice(stco);
        
        // Wrap in stbl
        let mut stbl = BytesMut::new();
        stbl.put_u32(8 + buf.len() as u32);
        stbl.put_slice(b"stbl");
        stbl.put_slice(&buf);
        
        Ok(stbl.freeze())
    }
    
    fn generate_stsd(&self) -> Result<Bytes> {
        let mut buf = BytesMut::with_capacity(150);
        
        buf.put_u32(0); // size placeholder
        buf.put_slice(b"stsd");
        buf.put_u32(0);  // version and flags
        buf.put_u32(1);  // entry count
        
        // avc1 sample entry
        let avc1_start = buf.len();
        buf.put_u32(0); // size placeholder
        buf.put_slice(b"avc1");
        buf.put_slice(&[0; 6]); // reserved
        buf.put_u16(1); // data reference index
        buf.put_u16(0); // pre-defined
        buf.put_u16(0); // reserved
        buf.put_slice(&[0; 12]); // pre-defined
        buf.put_u16(0); // width
        buf.put_u16(0); // height
        buf.put_u32(0x00480000); // horizresolution
        buf.put_u32(0x00480000); // vertresolution
        buf.put_u32(0); // reserved
        buf.put_u16(1); // frame count
        buf.put_slice(&[0; 32]); // compressor name
        buf.put_u16(24); // depth
        buf.put_i16(-1); // pre-defined
        
        // avcC box
        let avcc = self.generate_avcc()?;
        buf.put_slice(&avcc);
        
        // Fix sizes
        let avc1_size = buf.len() - avc1_start;
        let buf_slice = &mut buf[avc1_start..avc1_start + 4];
        buf_slice[..4].copy_from_slice(&(avc1_size as u32).to_be_bytes());
        
        let total_size = buf.len() as u32;
        buf[..4].copy_from_slice(&total_size.to_be_bytes());
        
        Ok(buf.freeze())
    }
    
    fn generate_avcc(&self) -> Result<Bytes> {
        let mut buf = BytesMut::with_capacity(50);
        
        buf.put_u32(0); // size placeholder
        buf.put_slice(b"avcC");
        
        // AVC decoder config
        buf.put_u8(1); // configuration version
        buf.put_u8(0x42); // AVC profile indication (Baseline)
        buf.put_u8(0x00); // AVC profile compatibility
        buf.put_u8(0x1F); // AVC level indication (3.1)
        buf.put_u8(0xFF); // length size minus one (3 bytes)
        buf.put_u8(1); // num of sps
        buf.put_u16(0); // sps length (empty for now)
        buf.put_u8(1); // num of pps
        buf.put_u16(0); // pps length (empty for now)
        
        let size = buf.len() as u32;
        buf[..4].copy_from_slice(&size.to_be_bytes());
        
        Ok(buf.freeze())
    }
    
    /// Generate media segment for current packets
    fn generate_media_segment(&self, packets: &[RtpPacket], sequence_number: u32) -> Result<Bytes> {
        let mut buf = BytesMut::with_capacity(packets.len() * 1500);
        
        // moof - movie fragment
        let moof = self.generate_moof(packets, sequence_number)?;
        buf.put_slice(&moof);
        
        // mdat - media data
        let mdat = self.generate_mdat(packets);
        buf.put_slice(&mdat);
        
        Ok(buf.freeze())
    }
    
    fn generate_moof(&self, packets: &[RtpPacket], sequence_number: u32) -> Result<Bytes> {
        let mut buf = BytesMut::with_capacity(256);
        
        // mfhd - movie fragment header
        let mfhd = self.generate_mfhd(sequence_number);
        buf.put_slice(&mfhd);
        
        // traf - track fragment
        let traf = self.generate_traf(packets, sequence_number)?;
        buf.put_slice(&traf);
        
        // Wrap in moof
        let mut moof = BytesMut::new();
        moof.put_u32(8 + buf.len() as u32);
        moof.put_slice(b"moof");
        moof.put_slice(&buf);
        
        Ok(moof.freeze())
    }
    
    fn generate_mfhd(&self, sequence_number: u32) -> Bytes {
        let mut buf = BytesMut::with_capacity(16);
        
        buf.put_u32(16); // size
        buf.put_slice(b"mfhd");
        buf.put_u32(0);  // version and flags
        buf.put_u32(sequence_number); // sequence number
        
        buf.freeze()
    }
    
    fn generate_traf(&self, packets: &[RtpPacket], _sequence_number: u32) -> Result<Bytes> {
        let mut buf = BytesMut::with_capacity(128);
        
        // tfhd - track fragment header
        let tfhd = self.generate_tfhd();
        buf.put_slice(&tfhd);
        
        // trun - track fragment run
        let trun = self.generate_trun(packets)?;
        buf.put_slice(&trun);
        
        // Wrap in traf
        let mut traf = BytesMut::new();
        traf.put_u32(8 + buf.len() as u32);
        traf.put_slice(b"traf");
        traf.put_slice(&buf);
        
        Ok(traf.freeze())
    }
    
    fn generate_tfhd(&self) -> Bytes {
        let mut buf = BytesMut::with_capacity(24);
        
        buf.put_u32(24); // size
        buf.put_slice(b"tfhd");
        buf.put_u32(0x00020); // version and flags (default-base-is-moof)
        buf.put_u32(state.track_id); // track ID
        buf.put_u32(0); // base data offset
        
        buf.freeze()
    }
    
    fn generate_trun(&self, packets: &[RtpPacket]) -> Result<Bytes> {
        let mut buf = BytesMut::with_capacity(64 + packets.len() * 16);
        
        let flags = 0x000F01; // data-offset-present, sample-duration-present, sample-size-present, sample-flags-present
        let trun_size = 12 + packets.len() * 16;
        
        buf.put_u32(trun_size as u32);
        buf.put_slice(b"trun");
        buf.put_u32(flags);
        buf.put_u32(packets.len() as u32);
        buf.put_u32(0); // data offset (relative to moof)
        
        let mut timestamp = state.base_timestamp.unwrap_or(0);
        
        for packet in packets {
            buf.put_u32(packet.payload.len() as u32); // sample duration (approximate)
            buf.put_u32(packet.payload.len() as u32); // sample size
            buf.put_u32(0x01010000); // sample flags (is leading, depends on, is depended on, has redundancy)
            buf.put_u32((timestamp & 0xFFFFFFFF) as u32); // sample composition time offset
            timestamp += 900; // Increment by ~10ms at 90kHz
        }
        
        Ok(buf.freeze())
    }
    
    fn generate_mdat(&self, packets: &[RtpPacket]) -> Bytes {
        let mut buf = BytesMut::with_capacity(packets.iter().map(|p| p.payload.len()).sum::<usize>() + 8);
        
        buf.put_u32(0); // size placeholder
        buf.put_slice(b"mdat");
        
        for packet in packets {
            buf.put_slice(&packet.payload);
        }
        
        let size = buf.len() as u32;
        buf[..4].copy_from_slice(&size.to_be_bytes());
        
        buf.freeze()
    }
}

// Helper to access track_id from state
mod state {
    use super::*;
    pub fn track_id() -> u32 { 1 }
}
