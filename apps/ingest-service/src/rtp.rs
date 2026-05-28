//! RTP packet handling and jitter buffer

use bytes::{Buf, BufMut, Bytes, BytesMut};
use dashmap::DashMap;
use parking_lot::RwLock;
use std::collections::{BTreeMap, VecDeque};
use std::net::SocketAddr;
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::net::UdpSocket;
use tokio::sync::mpsc;
use tracing::{debug, error, info, warn};

use crate::error::{Error, Result};
use crate::metrics;
use crate::rtsp::RtspSessionManager;

/// RTP packet header structure
#[derive(Debug, Clone)]
pub struct RtpHeader {
    pub version: u8,
    pub padding: bool,
    pub extension: bool,
    pub csrc_count: u8,
    pub marker: bool,
    pub payload_type: u8,
    pub sequence_number: u16,
    pub timestamp: u32,
    pub ssrc: u32,
    pub csrcs: Vec<u32>,
    pub extension_profile: Option<u16>,
    pub extension_payload: Option<Bytes>,
}

impl RtpHeader {
    /// Parse RTP header from bytes
    pub fn parse(data: &[u8]) -> Result<(Self, usize)> {
        if data.len() < 12 {
            return Err(Error::Rtp("Packet too short for RTP header".into()));
        }
        
        let first = data[0];
        let version = (first >> 6) & 0x03;
        let padding = (first >> 5) & 0x01 != 0;
        let extension = (first >> 4) & 0x01 != 0;
        let csrc_count = first & 0x0F;
        
        let second = data[1];
        let marker = (second >> 7) & 0x01 != 0;
        let payload_type = second & 0x7F;
        
        let sequence_number = u16::from_be_bytes([data[2], data[3]]);
        let timestamp = u32::from_be_bytes([data[4], data[5], data[6], data[7]]);
        let ssrc = u32::from_be_bytes([data[8], data[9], data[10], data[11]]);
        
        let mut offset = 12;
        let mut csrcs = Vec::with_capacity(csrc_count as usize);
        
        for _ in 0..csrc_count {
            if offset + 4 > data.len() {
                return Err(Error::Rtp("Packet too short for CSRC list".into()));
            }
            let csrc = u32::from_be_bytes([
                data[offset],
                data[offset + 1],
                data[offset + 2],
                data[offset + 3],
            ]);
            csrcs.push(csrc);
            offset += 4;
        }
        
        let mut extension_profile = None;
        let mut extension_payload = None;
        
        if extension {
            if offset + 4 > data.len() {
                return Err(Error::Rtp("Packet too short for extension header".into()));
            }
            extension_profile = Some(u16::from_be_bytes([data[offset], data[offset + 1]]));
            let ext_len = u16::from_be_bytes([data[offset + 2], data[offset + 3]]) as usize * 4;
            offset += 4;
            
            if offset + ext_len > data.len() {
                return Err(Error::Rtp("Packet too short for extension payload".into()));
            }
            extension_payload = Some(Bytes::copy_from_slice(&data[offset..offset + ext_len]));
            offset += ext_len;
        }
        
        let header = Self {
            version,
            padding,
            extension,
            csrc_count,
            marker,
            payload_type,
            sequence_number,
            timestamp,
            ssrc,
            csrcs,
            extension_profile,
            extension_payload,
        };
        
        Ok((header, offset))
    }
    
    /// Serialize RTP header to bytes
    pub fn serialize(&self, buf: &mut BytesMut) {
        let mut first = (self.version << 6) | self.csrc_count;
        if self.padding {
            first |= 0x20;
        }
        if self.extension {
            first |= 0x10;
        }
        buf.put_u8(first);
        
        let mut second = self.payload_type;
        if self.marker {
            second |= 0x80;
        }
        buf.put_u8(second);
        
        buf.put_u16(self.sequence_number);
        buf.put_u32(self.timestamp);
        buf.put_u32(self.ssrc);
        
        for csrc in &self.csrcs {
            buf.put_u32(*csrc);
        }
        
        if self.extension {
            if let Some(profile) = self.extension_profile {
                buf.put_u16(profile);
                if let Some(ref payload) = self.extension_payload {
                    let len_words = (payload.len() + 3) / 4;
                    buf.put_u16(len_words as u16);
                    buf.put_slice(payload);
                } else {
                    buf.put_u16(0);
                }
            }
        }
    }
}

/// RTP packet with metadata
#[derive(Debug, Clone)]
pub struct RtpPacket {
    pub header: RtpHeader,
    pub payload: Bytes,
    pub received_at: Instant,
    pub source_addr: Option<SocketAddr>,
}

impl RtpPacket {
    /// Parse RTP packet from bytes
    pub fn parse(data: &[u8], source_addr: Option<SocketAddr>) -> Result<Self> {
        let (header, header_len) = RtpHeader::parse(data)?;
        let payload = Bytes::copy_from_slice(&data[header_len..]);
        
        Ok(Self {
            header,
            payload,
            received_at: Instant::now(),
            source_addr,
        })
    }
    
    /// Serialize complete RTP packet
    pub fn serialize(&self, buf: &mut BytesMut) {
        self.header.serialize(buf);
        buf.put_slice(&self.payload);
    }
    
    /// Get total packet size
    pub fn size(&self) -> usize {
        12 + (self.header.csrc_count as usize * 4) + 
        if self.header.extension { 
            4 + self.header.extension_payload.as_ref().map(|p| p.len()).unwrap_or(0) 
        } else { 
            0 
        } + 
        self.payload.len()
    }
    
    /// Get latency since reception
    pub fn latency(&self) -> Duration {
        self.received_at.elapsed()
    }
}

/// Jitter buffer for reordering RTP packets
#[derive(Debug)]
pub struct JitterBuffer {
    max_size: usize,
    max_latency: Duration,
    buffers: DashMap<u32, StreamBuffer>,
    output_tx: mpsc::Sender<RtpPacket>,
    output_rx: Arc<RwLock<Option<mpsc::Receiver<RtpPacket>>>>,
}

#[derive(Debug)]
struct StreamBuffer {
    ssrc: u32,
    packets: BTreeMap<u16, RtpPacket>,
    expected_seq: u16,
    base_timestamp: Option<u32>,
    last_activity: Instant,
    loss_count: u64,
    reorder_count: u64,
}

impl StreamBuffer {
    fn new(ssrc: u32) -> Self {
        Self {
            ssrc,
            packets: BTreeMap::new(),
            expected_seq: 0,
            base_timestamp: None,
            last_activity: Instant::now(),
            loss_count: 0,
            reorder_count: 0,
        }
    }
}

impl JitterBuffer {
    /// Create a new jitter buffer
    pub fn new(max_size: usize, max_latency_ms: u64) -> Self {
        let (output_tx, output_rx) = mpsc::channel(1024);
        
        Self {
            max_size,
            max_latency: Duration::from_millis(max_latency_ms),
            buffers: DashMap::new(),
            output_tx,
            output_rx: Arc::new(RwLock::new(Some(output_rx))),
        }
    }
    
    /// Push an RTP packet into the buffer
    pub fn push(&self, packet: RtpPacket) -> Result<()> {
        let ssrc = packet.header.ssrc;
        let seq = packet.header.sequence_number;
        
        metrics::record_rtp_packet_received(ssrc, packet.header.payload_type, packet.size());
        
        let mut stream_buf = self.buffers.entry(ssrc).or_insert_with(|| StreamBuffer::new(ssrc));
        
        // Check buffer size
        if stream_buf.packets.len() >= self.max_size {
            metrics::record_rtp_packet_dropped(ssrc, "buffer_full");
            return Err(Error::BufferOverflow(format!(
                "Jitter buffer full for SSRC {}",
                ssrc
            )));
        }
        
        // Check latency
        if packet.latency() > self.max_latency {
            metrics::record_rtp_packet_dropped(ssrc, "too_old");
            return Ok(()); // Silently drop old packets
        }
        
        // Initialize expected sequence number
        if stream_buf.base_timestamp.is_none() {
            stream_buf.expected_seq = seq;
            stream_buf.base_timestamp = Some(packet.header.timestamp);
        }
        
        // Check if this is a reordered packet
        let is_reordered = seq != stream_buf.expected_seq && stream_buf.packets.contains_key(&seq);
        
        // Insert packet
        stream_buf.packets.insert(seq, packet);
        
        if is_reordered {
            stream_buf.reorder_count += 1;
            metrics::record_packet_reordered(ssrc, seq.wrapping_sub(stream_buf.expected_seq));
        }
        
        stream_buf.last_activity = Instant::now();
        
        // Update metrics
        metrics::record_buffer_occupancy(
            ssrc,
            stream_buf.packets.len(),
            stream_buf.packets.values().map(|p| p.latency().as_millis() as f64).max().unwrap_or(0.0),
        );
        
        Ok(())
    }
    
    /// Pop the next ordered packet for a stream
    pub fn pop(&self, ssrc: u32) -> Option<RtpPacket> {
        let mut stream_buf = self.buffers.get_mut(&ssrc)?;
        
        if let Some((seq, _)) = stream_buf.packets.first_key_value() {
            let expected = stream_buf.expected_seq;
            
            // Check for packet loss
            if *seq != expected && !stream_buf.packets.is_empty() {
                let gap = seq.wrapping_sub(expected);
                if gap > 0 && gap < 32768 {
                    stream_buf.loss_count += gap as u64;
                    metrics::record_packet_loss(ssrc, gap);
                }
            }
            
            let packet = stream_buf.packets.pop_first()?;
            stream_buf.expected_seq = packet.0.wrapping_add(1);
            
            Some(packet.1)
        } else {
            None
        }
    }
    
    /// Get receiver channel
    pub fn subscribe(&self) -> Option<mpsc::Receiver<RtpPacket>> {
        self.output_rx.write().take()
    }
    
    /// Receive loop - reads from UDP socket
    pub async fn receive_loop(&self, rtsp_manager: Arc<RtspSessionManager>) -> Result<()> {
        let socket = UdpSocket::bind("0.0.0.0:0").await?;
        let mut buf = vec![0u8; 65536];
        
        info!("RTP receiver listening on {}", socket.local_addr()?);
        
        loop {
            let (n, addr) = socket.recv_from(&mut buf).await?;
            
            match RtpPacket::parse(&buf[..n], Some(addr)) {
                Ok(packet) => {
                    // Try to associate with RTSP session
                    let ssrc = packet.header.ssrc;
                    
                    if let Err(e) = self.push(packet) {
                        warn!("Failed to push RTP packet: {}", e);
                    }
                }
                Err(e) => {
                    debug!("Failed to parse RTP packet from {}: {}", addr, e);
                }
            }
        }
    }
    
    /// Clean up stale streams
    pub fn cleanup_stale_streams(&self, timeout: Duration) -> usize {
        let mut removed = 0;
        
        self.buffers.retain(|_, stream_buf| {
            let keep = stream_buf.last_activity.elapsed() < timeout;
            if !keep {
                removed += 1;
            }
            keep
        });
        
        removed
    }
    
    /// Get statistics for a stream
    pub fn get_stats(&self, ssrc: u32) -> Option<StreamStats> {
        let stream_buf = self.buffers.get(&ssrc)?;
        
        Some(StreamStats {
            ssrc,
            packet_count: stream_buf.packets.len(),
            expected_seq: stream_buf.expected_seq,
            loss_count: stream_buf.loss_count,
            reorder_count: stream_buf.reorder_count,
            last_activity: stream_buf.last_activity.elapsed(),
        })
    }
}

/// Statistics for an RTP stream
#[derive(Debug, Clone)]
pub struct StreamStats {
    pub ssrc: u32,
    pub packet_count: usize,
    pub expected_seq: u16,
    pub loss_count: u64,
    pub reorder_count: u64,
    pub last_activity: Duration,
}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_rtp_header_parse() {
        let mut buf = BytesMut::with_capacity(100);
        buf.put_u8(0x80); // Version 2, no padding, no extension, 0 CSRCs
        buf.put_u8(96);   // Payload type 96, no marker
        buf.put_u16(1);   // Sequence number
        buf.put_u32(1000); // Timestamp
        buf.put_u32(0x12345678); // SSRC
        
        let (header, offset) = RtpHeader::parse(&buf).unwrap();
        
        assert_eq!(header.version, 2);
        assert!(!header.padding);
        assert!(!header.extension);
        assert_eq!(header.csrc_count, 0);
        assert!(!header.marker);
        assert_eq!(header.payload_type, 96);
        assert_eq!(header.sequence_number, 1);
        assert_eq!(header.timestamp, 1000);
        assert_eq!(header.ssrc, 0x12345678);
        assert_eq!(offset, 12);
    }
    
    #[test]
    fn test_jitter_buffer_ordering() {
        let buffer = JitterBuffer::new(100, 500);
        
        // Insert packets out of order
        let mut pkt1 = create_test_packet(0x12345678, 2, vec![1, 2, 3]);
        pkt1.header.timestamp = 1000;
        buffer.push(pkt1).unwrap();
        
        let mut pkt2 = create_test_packet(0x12345678, 1, vec![4, 5, 6]);
        pkt2.header.timestamp = 1000;
        buffer.push(pkt2).unwrap();
        
        // Should pop in order
        let popped = buffer.pop(0x12345678).unwrap();
        assert_eq!(popped.header.sequence_number, 1);
        
        let popped = buffer.pop(0x12345678).unwrap();
        assert_eq!(popped.header.sequence_number, 2);
    }
    
    fn create_test_packet(ssrc: u32, seq: u16, payload: Vec<u8>) -> RtpPacket {
        RtpPacket {
            header: RtpHeader {
                version: 2,
                padding: false,
                extension: false,
                csrc_count: 0,
                marker: false,
                payload_type: 96,
                sequence_number: seq,
                timestamp: 0,
                ssrc,
                csrcs: vec![],
                extension_profile: None,
                extension_payload: None,
            },
            payload: Bytes::from(payload),
            received_at: Instant::now(),
            source_addr: None,
        }
    }
}
