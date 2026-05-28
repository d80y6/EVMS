//! WebRTC module - re-exports and additional utilities

pub mod signaling;

pub use signaling::{
    SignalingManager,
    PeerConnection,
    PeerState,
    SessionDescription,
    IceCandidate,
    SignalingMessage,
};
