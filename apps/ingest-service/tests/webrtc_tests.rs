use ingest_service::webrtc::{WebrtcSignaling, PeerState, SdpOffer, SdpAnswer, IceCandidate};

#[test]
fn test_peer_state_default() {
    let state = PeerState::default();
    assert!(state.session_id.is_empty());
    assert!(state.remote_sdp.is_none());
    assert!(state.local_sdp.is_none());
    assert!(!state.ice_gathering_complete);
    assert!(!state.connection_established);
}

#[test]
fn test_sdp_offer_creation() {
    let offer = SdpOffer {
        sdp: "v=0\r\no=- 12345 1 IN IP4 127.0.0.1\r\n".to_string(),
        ice_ufrag: "abc123".to_string(),
        ice_pwd: "xyz789secret".to_string(),
        fingerprint: "sha-256 AA:BB:CC:DD:EE:FF".to_string(),
        candidates: vec![],
    };
    
    assert!(offer.sdp.contains("v=0"));
    assert_eq!(offer.ice_ufrag, "abc123");
    assert_eq!(offer.ice_pwd, "xyz789secret");
}

#[test]
fn test_sdp_answer_creation() {
    let answer = SdpAnswer {
        sdp: "v=0\r\no=- 67890 1 IN IP4 127.0.0.1\r\n".to_string(),
        ice_ufrag: "def456".to_string(),
        ice_pwd: "uvw321secret".to_string(),
        fingerprint: "sha-256 11:22:33:44:55:66".to_string(),
        candidates: vec![],
    };
    
    assert!(answer.sdp.contains("v=0"));
    assert_eq!(answer.ice_ufrag, "def456");
}

#[test]
fn test_ice_candidate_creation() {
    let candidate = IceCandidate {
        foundation: "1".to_string(),
        component_id: 1,
        protocol: "udp".to_string(),
        priority: 2130706431,
        address: "192.168.1.100".to_string(),
        port: 50000,
        typ: "host".to_string(),
        generation: 0,
    };
    
    assert_eq!(candidate.foundation, "1");
    assert_eq!(candidate.component_id, 1);
    assert_eq!(candidate.protocol, "udp");
    assert_eq!(candidate.address, "192.168.1.100");
    assert_eq!(candidate.port, 50000);
}

#[test]
fn test_ice_candidate_to_string() {
    let candidate = IceCandidate {
        foundation: "1".to_string(),
        component_id: 1,
        protocol: "udp".to_string(),
        priority: 2130706431,
        address: "192.168.1.100".to_string(),
        port: 50000,
        typ: "host".to_string(),
        generation: 0,
    };
    
    let candidate_str = candidate.to_string();
    assert!(candidate_str.contains("1 1 udp"));
    assert!(candidate_str.contains("2130706431"));
    assert!(candidate_str.contains("192.168.1.100"));
    assert!(candidate_str.contains("50000"));
    assert!(candidate_str.contains("typ host"));
}

#[test]
fn test_webrtc_signaling_create() {
    let signaling = WebrtcSignaling::new();
    
    assert!(signaling.get_peers().is_empty());
    assert_eq!(signaling.get_peers().len(), 0);
}

#[test]
fn test_webrtc_signaling_create_offer() {
    let mut signaling = WebrtcSignaling::new();
    
    let result = signaling.create_offer("test-peer-1");
    assert!(result.is_ok());
    
    let offer = result.unwrap();
    assert!(!offer.sdp.is_empty());
    assert!(!offer.ice_ufrag.is_empty());
    assert!(!offer.ice_pwd.is_empty());
    
    // Verify peer was created
    let peers = signaling.get_peers();
    assert!(peers.contains_key("test-peer-1"));
}

#[test]
fn test_webrtc_signaling_set_answer() {
    let mut signaling = WebrtcSignaling::new();
    
    // Create offer first
    signaling.create_offer("test-peer-2").unwrap();
    
    // Set answer
    let answer = SdpAnswer {
        sdp: "v=0\r\no=- answer 1 IN IP4 127.0.0.1\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n".to_string(),
        ice_ufrag: "answerUfrag".to_string(),
        ice_pwd: "answerPwd".to_string(),
        fingerprint: "sha-256 FF:EE:DD:CC:BB:AA".to_string(),
        candidates: vec![],
    };
    
    let result = signaling.set_answer("test-peer-2", answer);
    assert!(result.is_ok());
    
    // Verify peer has remote SDP
    let peers = signaling.get_peers();
    let peer = peers.get("test-peer-2").unwrap();
    assert!(peer.remote_sdp.is_some());
}

#[test]
fn test_webrtc_signaling_add_ice_candidate() {
    let mut signaling = WebrtcSignaling::new();
    
    // Create peer
    signaling.create_offer("test-peer-3").unwrap();
    
    // Add ICE candidate
    let candidate = IceCandidate {
        foundation: "1".to_string(),
        component_id: 1,
        protocol: "udp".to_string(),
        priority: 2130706431,
        address: "10.0.0.1".to_string(),
        port: 45678,
        typ: "host".to_string(),
        generation: 0,
    };
    
    let result = signaling.add_ice_candidate("test-peer-3", candidate);
    assert!(result.is_ok());
}

#[test]
fn test_webrtc_signaling_close_peer() {
    let mut signaling = WebrtcSignaling::new();
    
    // Create peer
    signaling.create_offer("test-peer-4").unwrap();
    assert_eq!(signaling.get_peers().len(), 1);
    
    // Close peer
    let result = signaling.close_peer("test-peer-4");
    assert!(result.is_ok());
    
    // Verify peer is removed
    assert!(signaling.get_peers().is_empty());
}

#[test]
fn test_webrtc_signaling_nonexistent_peer() {
    let mut signaling = WebrtcSignaling::new();
    
    // Try to set answer for non-existent peer
    let answer = SdpAnswer {
        sdp: "v=0\r\n".to_string(),
        ice_ufrag: "ufrag".to_string(),
        ice_pwd: "pwd".to_string(),
        fingerprint: "sha-256 AA:BB".to_string(),
        candidates: vec![],
    };
    
    let result = signaling.set_answer("nonexistent", answer);
    assert!(result.is_err());
}

#[test]
fn test_parse_sdp_fingerprint() {
    let sdp = r#"v=0
o=- 12345 1 IN IP4 127.0.0.1
a=fingerprint:sha-256 AA:BB:CC:DD:EE:FF:11:22:33:44:55:66:77:88:99:00:AA:BB:CC:DD:EE:FF:11:22:33:44:55:66
m=video 9 UDP/TLS/RTP/SAVPF 96
"#;
    
    let fingerprint = ingest_service::webrtc::parse_fingerprint_from_sdp(sdp);
    assert!(fingerprint.is_some());
    let fp = fingerprint.unwrap();
    assert!(fp.contains("AA:BB:CC:DD"));
}

#[test]
fn test_parse_sdp_ice_credentials() {
    let sdp = r#"v=0
o=- 12345 1 IN IP4 127.0.0.1
a=ice-ufrag:testUfrag123
a=ice-pwd:testPasswordSecret456
m=video 9 UDP/TLS/RTP/SAVPF 96
"#;
    
    let (ufrag, pwd) = ingest_service::webrtc::parse_ice_credentials(sdp);
    assert_eq!(ufrag, Some("testUfrag123".to_string()));
    assert_eq!(pwd, Some("testPasswordSecret456".to_string()));
}

#[test]
fn test_parse_ice_candidate_line() {
    let line = "a=candidate:1 1 udp 2130706431 192.168.1.100 50000 typ host generation 0";
    
    let candidate = ingest_service::webrtc::parse_ice_candidate_line(line);
    assert!(candidate.is_some());
    
    let c = candidate.unwrap();
    assert_eq!(c.foundation, "1");
    assert_eq!(c.component_id, 1);
    assert_eq!(c.protocol, "udp");
    assert_eq!(c.priority, 2130706431);
    assert_eq!(c.address, "192.168.1.100");
    assert_eq!(c.port, 50000);
    assert_eq!(c.typ, "host");
}

#[tokio::test]
async fn test_signaling_concurrent_operations() {
    let signaling = std::sync::Arc::new(tokio::sync::Mutex::new(WebrtcSignaling::new()));
    
    let mut handles = vec![];
    
    // Create multiple peers concurrently
    for i in 0..5 {
        let sig = signaling.clone();
        let handle = tokio::spawn(async move {
            let mut s = sig.lock().await;
            s.create_offer(&format!("peer-{}", i))
        });
        handles.push(handle);
    }
    
    for handle in handles {
        let result = handle.await.unwrap();
        assert!(result.is_ok());
    }
    
    let sig = signaling.lock().await;
    assert_eq!(sig.get_peers().len(), 5);
}
