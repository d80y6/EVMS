use ingest_service::rtp::{RtpHeader, JitterBuffer, JitterBufferConfig};
use ingest_service::rtcp::{RtcpPacket, RtcpSdes, RtcpSr};
use bytes::BytesMut;
use std::time::Duration;

#[test]
fn test_rtp_header_serialize_deserialize() {
    let header = RtpHeader {
        version: 2,
        padding: false,
        extension: true,
        marker: true,
        payload_type: 96,
        sequence_number: 12345,
        timestamp: 67890,
        ssrc: 0x12345678,
        csrcs: vec![0x11111111, 0x22222222],
        extension_profile: 0xBEDE,
        extension_payload: vec![0x01, 0x02, 0x03, 0x04],
    };

    let mut buf = BytesMut::new();
    header.serialize(&mut buf).unwrap();
    
    let parsed = RtpHeader::parse(&buf).unwrap();
    
    assert_eq!(parsed.version, 2);
    assert_eq!(parsed.padding, false);
    assert_eq!(parsed.extension, true);
    assert_eq!(parsed.marker, true);
    assert_eq!(parsed.payload_type, 96);
    assert_eq!(parsed.sequence_number, 12345);
    assert_eq!(parsed.timestamp, 67890);
    assert_eq!(parsed.ssrc, 0x12345678);
    assert_eq!(parsed.csrcs.len(), 2);
    assert_eq!(parsed.csrcs[0], 0x11111111);
    assert_eq!(parsed.csrcs[1], 0x22222222);
    assert_eq!(parsed.extension_profile, 0xBEDE);
    assert_eq!(parsed.extension_payload, vec![0x01, 0x02, 0x03, 0x04]);
}

#[test]
fn test_rtp_header_minimal() {
    let header = RtpHeader {
        version: 2,
        padding: false,
        extension: false,
        marker: false,
        payload_type: 96,
        sequence_number: 100,
        timestamp: 200,
        ssrc: 0xAABBCCDD,
        csrcs: vec![],
        extension_profile: 0,
        extension_payload: vec![],
    };

    let mut buf = BytesMut::new();
    header.serialize(&mut buf).unwrap();
    
    // Minimal header is 12 bytes
    assert_eq!(buf.len(), 12);
    
    let parsed = RtpHeader::parse(&buf).unwrap();
    assert_eq!(parsed.sequence_number, 100);
    assert_eq!(parsed.timestamp, 200);
}

#[test]
fn test_jitter_buffer_insert_ordered() {
    let config = JitterBufferConfig {
        max_size: 1000,
        max_delay_ms: 500,
    };
    let mut buffer = JitterBuffer::new(config);
    
    // Insert packets in order
    for i in 0..10 {
        let packet = vec![i as u8; 100];
        buffer.insert(i, packet);
    }
    
    // Should be able to pop all packets in order
    for i in 0..10 {
        let (seq, data) = buffer.pop_available().unwrap();
        assert_eq!(seq, i);
        assert_eq!(data.len(), 100);
        assert_eq!(data[0], i as u8);
    }
    
    assert!(buffer.pop_available().is_none());
}

#[test]
fn test_jitter_buffer_insert_out_of_order() {
    let config = JitterBufferConfig {
        max_size: 1000,
        max_delay_ms: 500,
    };
    let mut buffer = JitterBuffer::new(config);
    
    // Insert packets out of order: 5, 3, 1, 4, 2, 0
    let order = vec![5, 3, 1, 4, 2, 0];
    for &seq in &order {
        let packet = vec![seq as u8; 100];
        buffer.insert(seq, packet);
    }
    
    // Should pop in correct order
    for i in 0..6 {
        let (seq, data) = buffer.pop_available().unwrap();
        assert_eq!(seq, i);
        assert_eq!(data[0], i as u8);
    }
}

#[test]
fn test_jitter_buffer_detect_loss() {
    let config = JitterBufferConfig {
        max_size: 1000,
        max_delay_ms: 500,
    };
    let mut buffer = JitterBuffer::new(config);
    
    // Insert packets 0, 1, 3, 4 (missing 2)
    for &seq in &[0, 1, 3, 4] {
        let packet = vec![seq as u8; 100];
        buffer.insert(seq, packet);
    }
    
    let stats = buffer.get_stats();
    assert_eq!(stats.packets_received, 4);
    assert_eq!(stats.packets_lost, 1);
    assert_eq!(stats.expected_next_seq, 2);
}

#[test]
fn test_jitter_buffer_max_size() {
    let config = JitterBufferConfig {
        max_size: 5,
        max_delay_ms: 500,
    };
    let mut buffer = JitterBuffer::new(config);
    
    // Insert more than max_size packets
    for i in 0..10 {
        let packet = vec![i as u8; 100];
        buffer.insert(i, packet);
    }
    
    // Buffer should have dropped old packets
    let stats = buffer.get_stats();
    assert!(stats.packets_dropped > 0);
}

#[test]
fn test_rtcp_sr_parse() {
    // Build a minimal Sender Report packet
    let mut buf = BytesMut::new();
    buf.extend_from_slice(&[
        0x80, 0xC8, 0x00, 0x0C, // V=2, P=0, RC=0, PT=SR(200), length=12
        0x12, 0x34, 0x56, 0x78, // SSRC
        0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, // NTP timestamp
        0x00, 0x00, 0x00, 0x01, // RTP timestamp
        0x00, 0x00, 0x00, 0x00, // packet count
        0x00, 0x00, 0x00, 0x00, // octet count
    ]);
    
    let packets = RtcpPacket::parse_compound(&buf).unwrap();
    assert_eq!(packets.len(), 1);
    
    if let RtcpPacket::SenderReport(sr) = &packets[0] {
        assert_eq!(sr.ssrc, 0x12345678);
    } else {
        panic!("Expected SenderReport");
    }
}

#[test]
fn test_rtcp_rr_parse() {
    // Build a Receiver Report with one report block
    let mut buf = BytesMut::new();
    buf.extend_from_slice(&[
        0x81, 0xC9, 0x00, 0x07, // V=2, P=0, RC=1, PT=RR(201), length=7
        0x12, 0x34, 0x56, 0x78, // SSRC of receiver
        // Report block 1
        0xAA, 0xBB, 0xCC, 0xDD, // SSRC_1
        0x00, // fraction lost
        0x00, 0x00, 0x01, // cumulative lost (3 bytes)
        0x00, 0x00, 0x00, 0x00, // extended highest seq
        0x00, 0x00, 0x00, 0x00, // interarrival jitter
        0x00, 0x00, 0x00, 0x00, // LSR
        0x00, 0x00, 0x00, 0x00, // DLSR
    ]);
    
    let packets = RtcpPacket::parse_compound(&buf).unwrap();
    assert_eq!(packets.len(), 1);
    
    if let RtcpPacket::ReceiverReport(rr) = &packets[0] {
        assert_eq!(rr.ssrc, 0x12345678);
        assert_eq!(rr.reports.len(), 1);
        assert_eq!(rr.reports[0].ssrc, 0xAABBCCDD);
    } else {
        panic!("Expected ReceiverReport");
    }
}

#[test]
fn test_rtcp_sdes_parse() {
    // Build SDES with CNAME
    let mut buf = BytesMut::new();
    buf.extend_from_slice(&[
        0x81, 0xCA, 0x00, 0x05, // V=2, P=0, RC=1, PT=SDES(202), length=5
        0x12, 0x34, 0x56, 0x78, // SSRC
        0x01, 0x0A, b't', b'e', b's', b't', b'-', b'c', b'n', b'a', b'm', b'e', // CNAME item
        0x00, 0x00, 0x00, 0x00, // Padding to word boundary
    ]);
    
    let packets = RtcpPacket::parse_compound(&buf).unwrap();
    assert_eq!(packets.len(), 1);
    
    if let RtcpPacket::SourceDescription(sdes) = &packets[0] {
        assert_eq!(sdes.chunks.len(), 1);
        assert_eq!(sdes.chunks[0].ssrc, 0x12345678);
        assert!(sdes.chunks[0].items.contains_key(&0x01));
    } else {
        panic!("Expected SourceDescription");
    }
}

#[tokio::test]
async fn test_jitter_buffer_timeout() {
    let config = JitterBufferConfig {
        max_size: 1000,
        max_delay_ms: 50, // Short timeout for testing
    };
    let mut buffer = JitterBuffer::new(config);
    
    // Insert packet 5 without 0-4
    buffer.insert(5, vec![5u8; 100]);
    
    // Should not be available immediately
    assert!(buffer.pop_available().is_none());
    
    // Wait for timeout
    tokio::time::sleep(Duration::from_millis(100)).await;
    
    // After timeout, should release what we have
    let result = buffer.pop_available();
    assert!(result.is_some());
}
