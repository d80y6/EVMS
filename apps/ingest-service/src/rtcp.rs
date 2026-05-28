//! RTCP packet handling and quality feedback

use bytes::{Buf, BufMut, Bytes, BytesMut};
use std::collections::HashMap;
use std::net::SocketAddr;
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::net::UdpSocket;
use tracing::{debug, error, info, warn};

use crate::error::{Error, Result};
use crate::metrics;
use crate::rtp::{JitterBuffer, RtpPacket};

/// RTCP packet types
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(u8)]
pub enum RtcpPacketType {
    SenderReport = 200,
    ReceiverReport = 201,
    SourceDescription = 202,
    Bye = 203,
    ApplicationDefined = 204,
    TransportLayerNack = 205,
    PayloadSpecificFeedback = 206,
}

impl TryFrom<u8> for RtcpPacketType {
    type Error = Error;
    
    fn try_from(value: u8) -> Result<Self> {
        match value {
            200 => Ok(Self::SenderReport),
            201 => Ok(Self::ReceiverReport),
            202 => Ok(Self::SourceDescription),
            203 => Ok(Self::Bye),
            204 => Ok(Self::ApplicationDefined),
            205 => Ok(Self::TransportLayerNack),
            206 => Ok(Self::PayloadSpecificFeedback),
            _ => Err(Error::Rtcp(format!("Unknown RTCP packet type: {}", value))),
        }
    }
}

/// RTCP Sender Report
#[derive(Debug, Clone)]
pub struct SenderReport {
    pub ssrc: u32,
    pub ntp_timestamp_secs: u32,
    pub ntp_timestamp_frac: u32,
    pub rtp_timestamp: u32,
    pub sender_packet_count: u32,
    pub sender_octet_count: u32,
    pub reports: Vec<ReceptionReport>,
}

/// RTCP Reception Report
#[derive(Debug, Clone)]
pub struct ReceptionReport {
    pub ssrc: u32,
    pub fraction_lost: u8,
    pub cumulative_lost: u32,
    pub highest_seq_received: u16,
    pub interarrival_jitter: u32,
    pub last_sr: u32,
    pub delay_since_last_sr: u32,
}

/// RTCP Source Description
#[derive(Debug, Clone)]
pub struct SourceDescription {
    pub chunks: Vec<SdesChunk>,
}

#[derive(Debug, Clone)]
pub struct SdesChunk {
    pub ssrc: u32,
    pub items: HashMap<SdesItemType, String>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(u8)]
pub enum SdesItemType {
    End = 0,
    CName = 1,
    Name = 2,
    Email = 3,
    Phone = 4,
    Loc = 5,
    Tool = 6,
    Note = 7,
    Priv = 8,
}

/// RTCP BYE
#[derive(Debug, Clone)]
pub struct Bye {
    pub ssrcs: Vec<u32>,
    pub reason: Option<String>,
}

/// RTCP Transport Layer NACK
#[derive(Debug, Clone)]
pub struct TransportLayerNack {
    pub sender_ssrc: u32,
    pub media_ssrc: u32,
    pub lost_packets: Vec<u16>,
}

/// RTCP Packet
#[derive(Debug, Clone)]
pub enum RtcpPacket {
    SenderReport(SenderReport),
    ReceiverReport {
        ssrc: u32,
        reports: Vec<ReceptionReport>,
    },
    SourceDescription(SourceDescription),
    Bye(Bye),
    ApplicationDefined {
        ssrc: u32,
        name: [u8; 4],
        data: Bytes,
    },
    TransportLayerNack(TransportLayerNack),
    PayloadSpecificFeedback {
        ssrc: u32,
        feedback_type: u8,
        data: Bytes,
    },
}

impl RtcpPacket {
    /// Parse RTCP packet from bytes
    pub fn parse(data: &[u8]) -> Result<Vec<Self>> {
        let mut packets = Vec::new();
        let mut offset = 0;
        
        while offset < data.len() {
            if offset + 4 > data.len() {
                break;
            }
            
            let first = data[offset];
            let version = (first >> 6) & 0x03;
            if version != 2 {
                return Err(Error::Rtcp(format!("Invalid RTCP version: {}", version)));
            }
            
            let padding = (first >> 5) & 0x01 != 0;
            let count = first & 0x1F;
            let packet_type = data[offset + 1];
            let length = u16::from_be_bytes([data[offset + 2], data[offset + 3]]) as usize * 4;
            
            if offset + 4 + length > data.len() {
                return Err(Error::Rtcp("RTCP packet length exceeds buffer".into()));
            }
            
            let packet_data = &data[offset + 4..offset + 4 + length];
            
            let packet = Self::parse_single(packet_type, count, packet_data)?;
            packets.push(packet);
            
            offset += 4 + length;
        }
        
        Ok(packets)
    }
    
    fn parse_single(packet_type: u8, count: u8, data: &[u8]) -> Result<Self> {
        let pt = RtcpPacketType::try_from(packet_type)?;
        
        match pt {
            RtcpPacketType::SenderReport => Self::parse_sender_report(data),
            RtcpPacketType::ReceiverReport => Self::parse_receiver_report(packet_type, count, data),
            RtcpPacketType::SourceDescription => Self::parse_source_description(count, data),
            RtcpPacketType::Bye => Self::parse_bye(count, data),
            RtcpPacketType::TransportLayerNack => Self::parse_nack(data),
            _ => Ok(RtcpPacket::ApplicationDefined {
                ssrc: 0,
                name: [0; 4],
                data: Bytes::copy_from_slice(data),
            }),
        }
    }
    
    fn parse_sender_report(data: &[u8]) -> Result<Self> {
        if data.len() < 24 {
            return Err(Error::Rtcp("SR too short".into()));
        }
        
        let ssrc = u32::from_be_bytes([data[0], data[1], data[2], data[3]]);
        let ntp_timestamp_secs = u32::from_be_bytes([data[4], data[5], data[6], data[7]]);
        let ntp_timestamp_frac = u32::from_be_bytes([data[8], data[9], data[10], data[11]]);
        let rtp_timestamp = u32::from_be_bytes([data[12], data[13], data[14], data[15]]);
        let sender_packet_count = u32::from_be_bytes([data[16], data[17], data[18], data[19]]);
        let sender_octet_count = u32::from_be_bytes([data[20], data[21], data[22], data[23]]);
        
        let mut reports = Vec::new();
        let mut offset = 24;
        
        while offset + 24 <= data.len() {
            let report = Self::parse_reception_report(&data[offset..])?;
            reports.push(report);
            offset += 24;
        }
        
        Ok(RtcpPacket::SenderReport(SenderReport {
            ssrc,
            ntp_timestamp_secs,
            ntp_timestamp_frac,
            rtp_timestamp,
            sender_packet_count,
            sender_octet_count,
            reports,
        }))
    }
    
    fn parse_reception_report(data: &[u8]) -> Result<ReceptionReport> {
        if data.len() < 24 {
            return Err(Error::Rtcp("Reception report too short".into()));
        }
        
        Ok(ReceptionReport {
            ssrc: u32::from_be_bytes([data[0], data[1], data[2], data[3]]),
            fraction_lost: data[4],
            cumulative_lost: u32::from_be_bytes([data[5], data[6], data[7]]) & 0xFFFFFF,
            highest_seq_received: u16::from_be_bytes([data[8], data[9]]),
            interarrival_jitter: u32::from_be_bytes([data[10], data[11], data[12], data[13]]),
            last_sr: u32::from_be_bytes([data[14], data[15], data[16], data[17]]),
            delay_since_last_sr: u32::from_be_bytes([data[18], data[19], data[20], data[21]]),
        })
    }
    
    fn parse_receiver_report(pt: u8, count: u8, data: &[u8]) -> Result<Self> {
        if data.len() < 4 {
            return Err(Error::Rtcp("RR too short".into()));
        }
        
        let ssrc = u32::from_be_bytes([data[0], data[1], data[2], data[3]]);
        let mut reports = Vec::new();
        let mut offset = 4;
        
        for _ in 0..count {
            if offset + 24 > data.len() {
                break;
            }
            let report = Self::parse_reception_report(&data[offset..])?;
            reports.push(report);
            offset += 24;
        }
        
        Ok(RtcpPacket::ReceiverReport { ssrc, reports })
    }
    
    fn parse_source_description(count: u8, data: &[u8]) -> Result<Self> {
        let mut chunks = Vec::new();
        let mut offset = 0;
        
        for _ in 0..count {
            if offset + 4 > data.len() {
                break;
            }
            
            let ssrc = u32::from_be_bytes([data[offset], data[offset + 1], data[offset + 2], data[offset + 3]]);
            offset += 4;
            
            let mut items = HashMap::new();
            
            while offset < data.len() {
                let item_type = data[offset];
                if item_type == 0 {
                    offset += 1;
                    break;
                }
                
                offset += 1; // Skip type
                
                if offset >= data.len() {
                    break;
                }
                
                let len = data[offset] as usize;
                offset += 1;
                
                if offset + len > data.len() {
                    break;
                }
                
                let value = String::from_utf8_lossy(&data[offset..offset + len]).to_string();
                offset += len;
                
                // Pad to 4-byte boundary
                while offset % 4 != 0 {
                    offset += 1;
                }
                
                if let Some(item_type) = Self::u8_to_sdes_type(item_type) {
                    items.insert(item_type, value);
                }
            }
            
            chunks.push(SdesChunk { ssrc, items });
        }
        
        Ok(RtcpPacket::SourceDescription(SourceDescription { chunks }))
    }
    
    fn u8_to_sdes_type(value: u8) -> Option<SdesItemType> {
        match value {
            0 => Some(SdesItemType::End),
            1 => Some(SdesItemType::CName),
            2 => Some(SdesItemType::Name),
            3 => Some(SdesItemType::Email),
            4 => Some(SdesItemType::Phone),
            5 => Some(SdesItemType::Loc),
            6 => Some(SdesItemType::Tool),
            7 => Some(SdesItemType::Note),
            8 => Some(SdesItemType::Priv),
            _ => None,
        }
    }
    
    fn parse_bye(count: u8, data: &[u8]) -> Result<Self> {
        let mut ssrcs = Vec::with_capacity(count as usize);
        let mut offset = 0;
        
        for _ in 0..count {
            if offset + 4 > data.len() {
                break;
            }
            let ssrc = u32::from_be_bytes([data[offset], data[offset + 1], data[offset + 2], data[offset + 3]]);
            ssrcs.push(ssrc);
            offset += 4;
        }
        
        let reason = if offset < data.len() {
            let len = data[offset] as usize;
            offset += 1;
            if offset + len <= data.len() {
                Some(String::from_utf8_lossy(&data[offset..offset + len]).to_string())
            } else {
                None
            }
        } else {
            None
        };
        
        Ok(RtcpPacket::Bye(Bye { ssrcs, reason }))
    }
    
    fn parse_nack(data: &[u8]) -> Result<Self> {
        if data.len() < 8 {
            return Err(Error::Rtcp("NACK too short".into()));
        }
        
        let sender_ssrc = u32::from_be_bytes([data[0], data[1], data[2], data[3]]);
        let media_ssrc = u32::from_be_bytes([data[4], data[5], data[6], data[7]]);
        
        let mut lost_packets = Vec::new();
        let mut offset = 8;
        
        while offset + 4 <= data.len() {
            let pid = u16::from_be_bytes([data[offset], data[offset + 1]]);
            let blp = u16::from_be_bytes([data[offset + 2], data[offset + 3]]);
            
            lost_packets.push(pid);
            
            for i in 0..16 {
                if (blp & (1 << i)) != 0 {
                    lost_packets.push(pid.wrapping_add(i as u16).wrapping_add(1));
                }
            }
            
            offset += 4;
        }
        
        Ok(RtcpPacket::TransportLayerNack(TransportLayerNack {
            sender_ssrc,
            media_ssrc,
            lost_packets,
        }))
    }
    
    /// Serialize RTCP packet to bytes
    pub fn serialize(&self, buf: &mut BytesMut) {
        match self {
            RtcpPacket::SenderReport(sr) => {
                buf.put_u8(0x80 | sr.reports.len() as u8);
                buf.put_u8(200);
                buf.put_u16((1 + sr.reports.len() as u16) * 6); // 6 words header + reports
                
                buf.put_u32(sr.ssrc);
                buf.put_u32(sr.ntp_timestamp_secs);
                buf.put_u32(sr.ntp_timestamp_frac);
                buf.put_u32(sr.rtp_timestamp);
                buf.put_u32(sr.sender_packet_count);
                buf.put_u32(sr.sender_octet_count);
                
                for report in &sr.reports {
                    Self::serialize_reception_report(report, buf);
                }
            }
            _ => {
                // TODO: Implement other packet types
            }
        }
    }
    
    fn serialize_reception_report(report: &ReceptionReport, buf: &mut BytesMut) {
        buf.put_u32(report.ssrc);
        
        let lost = report.cumulative_lost & 0xFFFFFF;
        buf.put_u8(report.fraction_lost);
        buf.put_u8((lost >> 16) as u8);
        buf.put_u8((lost >> 8) as u8);
        buf.put_u8(lost as u8);
        
        buf.put_u16(report.highest_seq_received);
        buf.put_u32(report.interarrival_jitter);
        buf.put_u32(report.last_sr);
        buf.put_u32(report.delay_since_last_sr);
    }
}

/// RTCP handler for processing control packets
#[derive(Debug)]
pub struct RtcpHandler {
    jitter_buffer: Arc<JitterBuffer>,
    stats: Arc<parking_lot::RwLock<HashMap<u32, RtcpStats>>>,
}

#[derive(Debug, Clone, Default)]
pub struct RtcpStats {
    pub packets_sent: u64,
    pub bytes_sent: u64,
    pub packets_received: u64,
    pub bytes_received: u64,
    pub packets_lost: u64,
    pub round_trip_time_ms: f64,
    pub jitter_ms: f64,
    pub last_sr_time: Option<Instant>,
    pub last_rr_time: Option<Instant>,
}

impl RtcpHandler {
    /// Create a new RTCP handler
    pub fn new(jitter_buffer: Arc<JitterBuffer>) -> Self {
        Self {
            jitter_buffer,
            stats: Arc::new(parking_lot::RwLock::new(HashMap::new())),
        }
    }
    
    /// Process incoming RTCP packets
    pub fn process(&self, data: &[u8], source_addr: SocketAddr) -> Result<()> {
        let packets = RtcpPacket::parse(data)?;
        
        for packet in packets {
            metrics::record_rtcp_packet_received(
                match &packet {
                    RtcpPacket::SenderReport(sr) => 200,
                    RtcpPacket::ReceiverReport { .. } => 201,
                    RtcpPacket::SourceDescription(_) => 202,
                    RtcpPacket::Bye(_) => 203,
                    RtcpPacket::ApplicationDefined { .. } => 204,
                    RtcpPacket::TransportLayerNack(_) => 205,
                    RtcpPacket::PayloadSpecificFeedback { .. } => 206,
                },
                data.len(),
            );
            
            match packet {
                RtcpPacket::SenderReport(sr) => {
                    self.handle_sender_report(sr, source_addr);
                }
                RtcpPacket::ReceiverReport { ssrc, reports } => {
                    self.handle_receiver_report(ssrc, reports, source_addr);
                }
                RtcpPacket::TransportLayerNack(nack) => {
                    self.handle_nack(nack, source_addr);
                }
                RtcpPacket::Bye(bye) => {
                    debug!("Received RTCP BYE from {:?}", source_addr);
                    for ssrc in bye.ssrcs {
                        let mut stats = self.stats.write();
                        stats.remove(&ssrc);
                    }
                }
                _ => {
                    debug!("Received unhandled RTCP packet from {:?}", source_addr);
                }
            }
        }
        
        Ok(())
    }
    
    fn handle_sender_report(&self, sr: SenderReport, _source_addr: SocketAddr) {
        let mut stats = self.stats.write();
        let entry = stats.entry(sr.ssrc).or_default();
        
        entry.packets_received = sr.sender_packet_count as u64;
        entry.bytes_received = sr.sender_octet_count as u64;
        entry.last_sr_time = Some(Instant::now());
        
        debug!(
            "RTCP SR: SSRC={}, packets={}, octets={}",
            sr.ssrc, sr.sender_packet_count, sr.sender_octet_count
        );
    }
    
    fn handle_receiver_report(&self, ssrc: u32, reports: Vec<ReceptionReport>, _source_addr: SocketAddr) {
        let mut stats = self.stats.write();
        let entry = stats.entry(ssrc).or_default();
        
        for report in reports {
            entry.packets_lost = report.cumulative_lost as u64;
            entry.jitter_ms = (report.interarrival_jitter as f64) / 90.0; // Assuming 90kHz clock
            
            // Calculate RTT if we have a recent SR
            if let Some(last_sr) = entry.last_sr_time {
                let dlsr = Duration::from_millis((report.delay_since_last_sr as f64 * 1000.0 / 65536.0) as u64);
                let now = Instant::now();
                if let Some(elapsed) = now.checked_duration_since(last_sr) {
                    if elapsed > dlsr {
                        entry.round_trip_time_ms = (elapsed - dlsr).as_millis() as f64;
                    }
                }
            }
        }
        
        entry.last_rr_time = Some(Instant::now());
        
        debug!(
            "RTCP RR: SSRC={}, loss={}, jitter={:.2}ms, rtt={:.2}ms",
            ssrc, entry.packets_lost, entry.jitter_ms, entry.round_trip_time_ms
        );
    }
    
    fn handle_nack(&self, nack: TransportLayerNack, _source_addr: SocketAddr) {
        warn!(
            "RTCP NACK: sender={}, media={}, lost_packets={:?}",
            nack.sender_ssrc, nack.media_ssrc, nack.lost_packets
        );
        
        // Could trigger retransmission logic here
    }
    
    /// Run the RTCP handler
    pub async fn run(&self) -> Result<()> {
        let socket = UdpSocket::bind("0.0.0.0:0").await?;
        let mut buf = vec![0u8; 65536];
        
        info!("RTCP handler listening on {}", socket.local_addr()?);
        
        loop {
            let (n, addr) = socket.recv_from(&mut buf).await?;
            
            if let Err(e) = self.process(&buf[..n], addr) {
                debug!("Failed to process RTCP packet: {}", e);
            }
        }
    }
    
    /// Get statistics for an SSRC
    pub fn get_stats(&self, ssrc: u32) -> Option<RtcpStats> {
        self.stats.read().get(&ssrc).cloned()
    }
    
    /// Send a receiver report
    pub async fn send_receiver_report(
        &self,
        socket: &UdpSocket,
        dest: SocketAddr,
        ssrc: u32,
        remote_ssrc: u32,
    ) -> Result<()> {
        let stats = self.stats.read();
        let entry = stats.get(&remote_ssrc).cloned().unwrap_or_default();
        drop(stats);
        
        let mut buf = BytesMut::with_capacity(28);
        
        // Receiver Report
        buf.put_u8(0x81); // Version 2, no padding, 1 report
        buf.put_u8(201);  // RR
        buf.put_u16(7);   // Length in 32-bit words
        
        buf.put_u32(ssrc);
        
        // Reception report block
        buf.put_u32(remote_ssrc);
        
        // Fraction lost and cumulative lost
        let fraction_lost = 0u8; // TODO: Calculate from actual loss
        let cumulative_lost = entry.packets_lost & 0xFFFFFF;
        buf.put_u8(fraction_lost);
        buf.put_u8((cumulative_lost >> 16) as u8);
        buf.put_u8((cumulative_lost >> 8) as u8);
        buf.put_u8(cumulative_lost as u8);
        
        buf.put_u16(0); // Highest sequence number received
        buf.put_u32((entry.jitter_ms * 90.0) as u32); // Jitter
        buf.put_u32(0); // Last SR timestamp
        buf.put_u32(0); // Delay since last SR
        
        socket.send_to(&buf, dest).await?;
        
        Ok(())
    }
}
