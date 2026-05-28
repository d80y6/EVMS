//! WebRTC signaling and peer connection management

use async_trait::async_trait;
use bytes::Bytes;
use parking_lot::RwLock;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::sync::{broadcast, mpsc};
use tracing::{debug, error, info, warn};
use uuid::Uuid;

use crate::error::{Error, Result};
use crate::metrics;
use crate::rtp::RtpPacket;

/// WebRTC session description
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SessionDescription {
    #[serde(rename = "type")]
    pub sd_type: String,
    pub sdp: String,
}

/// ICE candidate
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct IceCandidate {
    pub candidate: String,
    pub sdp_mid: String,
    pub sdp_mline_index: u16,
}

/// WebRTC signaling message
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type")]
pub enum SignalingMessage {
    #[serde(rename = "offer")]
    Offer { id: String, sdp: String },
    #[serde(rename = "answer")]
    Answer { id: String, sdp: String },
    #[serde(rename = "ice-candidate")]
    IceCandidate { id: String, candidate: IceCandidate },
    #[serde(rename = "ice-end")]
    IceEnd { id: String },
    #[serde(rename = "close")]
    Close { id: String },
}

/// WebRTC peer state
#[derive(Debug, Clone, PartialEq)]
pub enum PeerState {
    New,
    Connecting,
    Connected,
    Disconnected,
    Failed,
    Closed,
}

/// WebRTC peer connection
#[derive(Debug)]
pub struct PeerConnection {
    pub id: String,
    pub state: PeerState,
    pub created_at: Instant,
    pub last_activity: Instant,
    pub remote_sdp: Option<SessionDescription>,
    pub local_sdp: Option<SessionDescription>,
    pub ice_candidates: Vec<IceCandidate>,
    pub ssrc: Option<u32>,
    pub payload_type: Option<u8>,
}

impl PeerConnection {
    pub fn new() -> Self {
        Self {
            id: Uuid::new_v4().to_string(),
            state: PeerState::New,
            created_at: Instant::now(),
            last_activity: Instant::now(),
            remote_sdp: None,
            local_sdp: None,
            ice_candidates: Vec::new(),
            ssrc: None,
            payload_type: None,
        }
    }
    
    pub fn is_active(&self) -> bool {
        self.state == PeerState::Connected || self.state == PeerState::Connecting
    }
}

impl Default for PeerConnection {
    fn default() -> Self {
        Self::new()
    }
}

/// WebRTC signaling manager
#[derive(Debug)]
pub struct SignalingManager {
    peers: Arc<RwLock<HashMap<String, PeerConnection>>>,
    message_tx: mpsc::Sender<(String, SignalingMessage)>,
    message_rx: Arc<RwLock<Option<mpsc::Receiver<(String, SignalingMessage)>>>>,
    shutdown_tx: broadcast::Sender<()>,
    ice_servers: Vec<String>,
}

impl SignalingManager {
    /// Create a new signaling manager
    pub fn new(ice_servers: Vec<String>) -> Self {
        let (message_tx, message_rx) = mpsc::channel(1024);
        let (shutdown_tx, _) = broadcast::channel(16);
        
        Self {
            peers: Arc::new(RwLock::new(HashMap::new())),
            message_tx,
            message_rx: Arc::new(RwLock::new(Some(message_rx))),
            shutdown_tx,
            ice_servers,
        }
    }
    
    /// Get message receiver
    pub fn subscribe(&self) -> Option<mpsc::Receiver<(String, SignalingMessage)>> {
        self.message_rx.write().take()
    }
    
    /// Get shutdown receiver
    pub fn shutdown_rx(&self) -> broadcast::Receiver<()> {
        self.shutdown_tx.subscribe()
    }
    
    /// Create a new peer connection
    pub fn create_peer(&self) -> String {
        let peer = PeerConnection::new();
        let id = peer.id.clone();
        
        self.peers.write().insert(id.clone(), peer);
        
        debug!("Created peer connection {}", id);
        
        id
    }
    
    /// Get a peer connection
    pub fn get_peer(&self, id: &str) -> Option<PeerConnection> {
        self.peers.read().get(id).cloned()
    }
    
    /// Update peer state
    pub fn update_peer_state(&self, id: &str, state: PeerState) -> Result<()> {
        let mut peers = self.peers.write();
        if let Some(peer) = peers.get_mut(id) {
            peer.state = state;
            peer.last_activity = Instant::now();
            Ok(())
        } else {
            Err(Error::Webrtc(format!("Peer {} not found", id)))
        }
    }
    
    /// Set remote SDP
    pub fn set_remote_sdp(&self, id: &str, sdp: SessionDescription) -> Result<()> {
        let mut peers = self.peers.write();
        if let Some(peer) = peers.get_mut(id) {
            peer.remote_sdp = Some(sdp);
            peer.last_activity = Instant::now();
            Ok(())
        } else {
            Err(Error::Webrtc(format!("Peer {} not found", id)))
        }
    }
    
    /// Set local SDP
    pub fn set_local_sdp(&self, id: &str, sdp: SessionDescription) -> Result<()> {
        let mut peers = self.peers.write();
        if let Some(peer) = peers.get_mut(id) {
            peer.local_sdp = Some(sdp);
            peer.last_activity = Instant::now();
            Ok(())
        } else {
            Err(Error::Webrtc(format!("Peer {} not found", id)))
        }
    }
    
    /// Add ICE candidate
    pub fn add_ice_candidate(&self, id: &str, candidate: IceCandidate) -> Result<()> {
        let mut peers = self.peers.write();
        if let Some(peer) = peers.get_mut(id) {
            peer.ice_candidates.push(candidate);
            peer.last_activity = Instant::now();
            Ok(())
        } else {
            Err(Error::Webrtc(format!("Peer {} not found", id)))
        }
    }
    
    /// Remove a peer connection
    pub fn remove_peer(&self, id: &str) -> Option<PeerConnection> {
        let peer = self.peers.write().remove(id);
        if peer.is_some() {
            debug!("Removed peer connection {}", id);
        }
        peer
    }
    
    /// Create an offer for a peer
    pub fn create_offer(&self, id: &str) -> Result<SessionDescription> {
        metrics::record_webrtc_offer_created();
        
        // Generate a minimal SDP offer
        let sdp = self.generate_sdp_offer(id)?;
        
        self.set_local_sdp(id, sdp.clone())?;
        self.update_peer_state(id, PeerState::Connecting)?;
        
        Ok(sdp)
    }
    
    /// Handle incoming offer
    pub fn handle_offer(&self, id: &str, sdp: String) -> Result<SessionDescription> {
        let remote_sdp = SessionDescription {
            sd_type: "offer".to_string(),
            sdp: sdp.clone(),
        };
        
        self.set_remote_sdp(id, remote_sdp)?;
        
        // Create answer
        let answer = self.create_answer(id)?;
        
        self.update_peer_state(id, PeerState::Connected)?;
        metrics::record_webrtc_answer_received();
        
        Ok(answer)
    }
    
    /// Create an answer
    pub fn create_answer(&self, id: &str) -> Result<SessionDescription> {
        let sdp = self.generate_sdp_answer(id)?;
        
        let answer = SessionDescription {
            sd_type: "answer".to_string(),
            sdp: sdp.clone(),
        };
        
        self.set_local_sdp(id, answer.clone())?;
        
        Ok(answer)
    }
    
    /// Generate SDP offer
    fn generate_sdp_offer(&self, id: &str) -> Result<SessionDescription> {
        let sdp = format!(
            "v=0\r\n\
             o=- 0 0 IN IP4 127.0.0.1\r\n\
             s=WebRTC Streaming\r\n\
             c=IN IP4 0.0.0.0\r\n\
             t=0 0\r\n\
             a=ice-ufrag:{ufrag}\r\n\
             a=ice-pwd:{pwd}\r\n\
             a=fingerprint:sha-256 00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00\r\n\
             a=setup:actpass\r\n\
             a=sendonly\r\n\
             m=video 9 UDP/TLS/RTP/SAVPF 96\r\n\
             c=IN IP4 0.0.0.0\r\n\
             a=mid:video\r\n\
             a=rtpmap:96 H264/90000\r\n\
             a=fmtp:96 packetization-mode=1;profile-level-id=42e01f\r\n\
             a=ssrc:{ssrc} cname:webrtc-stream\r\n\
             a=rtcp-mux\r\n\
             a=ice-options:trickle\r\n",
            ufrag = Uuid::new_v4().as_simple(),
            pwd = Uuid::new_v4().as_simple(),
            ssrc = rand_u32()
        );
        
        Ok(SessionDescription {
            sd_type: "offer".to_string(),
            sdp,
        })
    }
    
    /// Generate SDP answer
    fn generate_sdp_answer(&self, id: &str) -> Result<SessionDescription> {
        let sdp = format!(
            "v=0\r\n\
             o=- 0 0 IN IP4 127.0.0.1\r\n\
             s=WebRTC Streaming\r\n\
             c=IN IP4 0.0.0.0\r\n\
             t=0 0\r\n\
             a=ice-ufrag:{ufrag}\r\n\
             a=ice-pwd:{pwd}\r\n\
             a=fingerprint:sha-256 00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00\r\n\
             a=setup:active\r\n\
             a=recvonly\r\n\
             m=video 9 UDP/TLS/RTP/SAVPF 96\r\n\
             c=IN IP4 0.0.0.0\r\n\
             a=mid:video\r\n\
             a=rtpmap:96 H264/90000\r\n\
             a=fmtp:96 packetization-mode=1;profile-level-id=42e01f\r\n\
             a=rtcp-mux\r\n",
            ufrag = Uuid::new_v4().as_simple(),
            pwd = Uuid::new_v4().as_simple()
        );
        
        Ok(SessionDescription {
            sd_type: "answer".to_string(),
            sdp,
        })
    }
    
    /// Clean up stale peers
    pub fn cleanup_stale_peers(&self, timeout: Duration) -> usize {
        let mut peers = self.peers.write();
        let before = peers.len();
        
        peers.retain(|_, peer| {
            peer.last_activity.elapsed() < timeout && peer.is_active()
        });
        
        let removed = before - peers.len();
        if removed > 0 {
            debug!("Cleaned up {} stale peer connections", removed);
        }
        removed
    }
    
    /// Get active peer count
    pub fn active_peer_count(&self) -> usize {
        self.peers.read().values().filter(|p| p.is_active()).count()
    }
    
    /// Shutdown all peers
    pub fn shutdown(&self) {
        let _ = self.shutdown_tx.send(());
        
        let mut peers = self.peers.write();
        for (_, peer) in peers.iter_mut() {
            peer.state = PeerState::Closed;
        }
    }
}

fn rand_u32() -> u32 {
    use std::time::{SystemTime, UNIX_EPOCH};
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .subsec_nanos()
}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_create_peer() {
        let manager = SignalingManager::new(vec![]);
        let id = manager.create_peer();
        
        let peer = manager.get_peer(&id).unwrap();
        assert_eq!(peer.state, PeerState::New);
        assert!(peer.id == id);
    }
    
    #[test]
    fn test_signaling_message_serialization() {
        let msg = SignalingMessage::Offer {
            id: "test-id".to_string(),
            sdp: "v=0\r\n...".to_string(),
        };
        
        let json = serde_json::to_string(&msg).unwrap();
        assert!(json.contains("\"type\":\"offer\""));
        
        let deserialized: SignalingMessage = serde_json::from_str(&json).unwrap();
        match deserialized {
            SignalingMessage::Offer { id, sdp } => {
                assert_eq!(id, "test-id");
            }
            _ => panic!("Wrong variant"),
        }
    }
}
