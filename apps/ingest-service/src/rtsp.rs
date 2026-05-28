//! RTSP session management

use bytes::{BufMut, BytesMut};
use parking_lot::RwLock;
use std::collections::HashMap;
use std::net::SocketAddr;
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::net::TcpListener;
use tokio::sync::broadcast;
use tracing::{debug, error, info, warn};
use uuid::Uuid;

use crate::error::{Error, Result};
use crate::metrics;

/// RTSP session state
#[derive(Debug, Clone)]
pub struct RtspSession {
    pub id: String,
    pub client_addr: SocketAddr,
    pub created_at: Instant,
    pub last_activity: Instant,
    pub stream_path: String,
    pub cseq: u32,
    pub ssrc: Option<u32>,
    pub transport: Option<String>,
    pub state: SessionState,
}

#[derive(Debug, Clone, PartialEq)]
pub enum SessionState {
    Initial,
    DescribeSent,
    SetupComplete,
    Playing,
    Recording,
    Terminated,
}

impl RtspSession {
    pub fn new(client_addr: SocketAddr, stream_path: String) -> Self {
        Self {
            id: Uuid::new_v4().to_string(),
            client_addr,
            created_at: Instant::now(),
            last_activity: Instant::now(),
            stream_path,
            cseq: 0,
            ssrc: None,
            transport: None,
            state: SessionState::Initial,
        }
    }
    
    pub fn next_cseq(&mut self) -> u32 {
        self.cseq += 1;
        self.cseq
    }
    
    pub fn is_active(&self) -> bool {
        self.state != SessionState::Terminated
    }
    
    pub fn elapsed_since_activity(&self) -> Duration {
        self.last_activity.elapsed()
    }
}

/// RTSP session manager
#[derive(Debug)]
pub struct RtspSessionManager {
    port: u16,
    sessions: Arc<RwLock<HashMap<String, RtspSession>>>,
    shutdown_rx: broadcast::Receiver<()>,
    listener: Option<TcpListener>,
}

impl RtspSessionManager {
    /// Create a new RTSP session manager
    pub fn new(port: u16, shutdown_rx: broadcast::Receiver<()>) -> Self {
        Self {
            port,
            sessions: Arc::new(RwLock::new(HashMap::new())),
            shutdown_rx,
            listener: None,
        }
    }
    
    /// Get the number of active sessions
    pub fn session_count(&self) -> usize {
        self.sessions.read().len()
    }
    
    /// Get a session by ID
    pub fn get_session(&self, id: &str) -> Option<RtspSession> {
        self.sessions.read().get(id).cloned()
    }
    
    /// Create a new session
    pub fn create_session(&self, client_addr: SocketAddr, stream_path: String) -> String {
        let session = RtspSession::new(client_addr, stream_path);
        let id = session.id.clone();
        
        metrics::record_rtsp_session_created();
        
        self.sessions.write().insert(id.clone(), session);
        id
    }
    
    /// Update a session
    pub fn update_session<F>(&self, id: &str, f: F) -> Result<()>
    where
        F: FnOnce(&mut RtspSession),
    {
        let mut sessions = self.sessions.write();
        if let Some(session) = sessions.get_mut(id) {
            f(session);
            session.last_activity = Instant::now();
            Ok(())
        } else {
            Err(Error::InvalidState(format!("Session {} not found", id)))
        }
    }
    
    /// Remove a session
    pub fn remove_session(&self, id: &str) -> Option<RtspSession> {
        let session = self.sessions.write().remove(id);
        if session.is_some() {
            metrics::record_rtsp_session_destroyed();
        }
        session
    }
    
    /// Clean up stale sessions
    pub fn cleanup_stale_sessions(&self, timeout: Duration) -> usize {
        let mut sessions = self.sessions.write();
        let before = sessions.len();
        
        sessions.retain(|_, session| {
            session.elapsed_since_activity() < timeout && session.is_active()
        });
        
        let removed = before - sessions.len();
        if removed > 0 {
            debug!("Cleaned up {} stale RTSP sessions", removed);
        }
        removed
    }
    
    /// Run the RTSP server
    pub async fn run(&self) -> Result<()> {
        let addr = format!("0.0.0.0:{}", self.port);
        self.listener = Some(
            TcpListener::bind(&addr)
                .await
                .map_err(|e| Error::Rtsp(format!("Failed to bind RTSP port {}: {}", self.port, e)))?,
        );
        
        info!("RTSP server listening on {}", addr);
        
        let listener = self.listener.as_ref().unwrap();
        let mut shutdown_rx = self.shutdown_rx.resubscribe();
        
        // Cleanup task
        let sessions = Arc::clone(&self.sessions);
        let mut cleanup_shutdown = self.shutdown_rx.resubscribe();
        tokio::spawn(async move {
            let mut interval = tokio::time::interval(Duration::from_secs(30));
            loop {
                tokio::select! {
                    _ = interval.tick() => {
                        let mut sessions_guard = sessions.write();
                        sessions_guard.retain(|_, s| {
                            s.elapsed_since_activity() < Duration::from_secs(300) && s.is_active()
                        });
                    }
                    _ = cleanup_shutdown.recv() => break,
                }
            }
        });
        
        loop {
            tokio::select! {
                result = listener.accept() => {
                    match result {
                        Ok((stream, addr)) => {
                            debug!("New RTSP connection from {}", addr);
                            let sessions = Arc::clone(&self.sessions);
                            tokio::spawn(async move {
                                if let Err(e) = handle_connection(stream, addr, sessions).await {
                                    error!("RTSP connection error: {}", e);
                                }
                            });
                        }
                        Err(e) => {
                            error!("Failed to accept RTSP connection: {}", e);
                        }
                    }
                }
                _ = shutdown_rx.recv() => {
                    info!("RTSP server shutting down");
                    break;
                }
            }
        }
        
        Ok(())
    }
    
    /// Get SSRC for a session
    pub fn get_ssrc_for_session(&self, session_id: &str) -> Option<u32> {
        self.sessions.read().get(session_id).and_then(|s| s.ssrc)
    }
}

/// Handle an RTSP connection
async fn handle_connection(
    stream: tokio::net::TcpStream,
    client_addr: SocketAddr,
    sessions: Arc<RwLock<HashMap<String, RtspSession>>>,
) -> Result<()> {
    use tokio::io::{AsyncReadExt, AsyncWriteExt};
    
    let (mut read_half, mut write_half) = stream.into_split();
    let mut buffer = BytesMut::with_capacity(4096);
    let mut current_session_id: Option<String> = None;
    
    loop {
        let mut buf = [0u8; 4096];
        tokio::select! {
            n = read_half.read(&mut buf) => {
                match n {
                    Ok(0) => break, // Connection closed
                    Ok(n) => {
                        buffer.put_slice(&buf[..n]);
                        
                        // Parse RTSP message
                        match parse_rtsp_message(&buffer) {
                            Ok(Some((method, path, headers, body))) => {
                                debug!("RTSP {} {} from {}", method, path, client_addr);
                                
                                // Handle RTSP methods
                                let response = handle_rtsp_method(
                                    &method,
                                    &path,
                                    &headers,
                                    &body,
                                    &mut current_session_id,
                                    client_addr,
                                    &sessions,
                                ).await?;
                                
                                let _ = write_half.write_all(response.as_bytes()).await;
                                buffer.clear();
                            }
                            Ok(None) => {
                                // Incomplete message, wait for more
                                continue;
                            }
                            Err(e) => {
                                warn!("Failed to parse RTSP message: {}", e);
                                buffer.clear();
                                continue;
                            }
                        }
                    }
                    Err(e) => {
                        error!("Read error: {}", e);
                        break;
                    }
                }
            }
        }
    }
    
    // Clean up session on disconnect
    if let Some(session_id) = current_session_id {
        if let Some(session) = sessions.write().remove(&session_id) {
            let mut updated = session;
            updated.state = SessionState::Terminated;
            metrics::record_rtsp_session_destroyed();
        }
    }
    
    Ok(())
}

/// Parse an RTSP message from buffer
fn parse_rtsp_message(
    buffer: &[u8],
) -> Result<Option<(String, String, HashMap<String, String>, String)>> {
    // Look for double CRLF
    let header_end = match buffer.windows(4).position(|w| w == b"\r\n\r\n") {
        Some(pos) => pos,
        None => return Ok(None), // Incomplete headers
    };
    
    let header_data = std::str::from_utf8(&buffer[..header_end])
        .map_err(|_| Error::Rtsp("Invalid UTF-8 in headers".into()))?;
    
    let mut lines = header_data.lines();
    let request_line = lines.next().ok_or_else(|| Error::Rtsp("Empty request".into()))?;
    
    // Parse request line: METHOD url RTSP/version
    let parts: Vec<&str> = request_line.split_whitespace().collect();
    if parts.len() != 3 {
        return Err(Error::Rtsp("Invalid request line".into()));
    }
    
    let method = parts[0].to_string();
    let path = parts[1].to_string();
    
    // Parse headers
    let mut headers = HashMap::new();
    for line in lines {
        if let Some((key, value)) = line.split_once(": ") {
            headers.insert(key.to_string(), value.to_string());
        }
    }
    
    // Check for content-length
    let body = if let Some(content_length) = headers.get("Content-Length") {
        let len: usize = content_length.parse().unwrap_or(0);
        let body_start = header_end + 4;
        if buffer.len() >= body_start + len {
            String::from_utf8_lossy(&buffer[body_start..body_start + len]).to_string()
        } else {
            return Ok(None); // Body incomplete
        }
    } else {
        String::new()
    };
    
    Ok(Some((method, path, headers, body)))
}

/// Handle an RTSP method
async fn handle_rtsp_method(
    method: &str,
    path: &str,
    headers: &HashMap<String, String>,
    _body: &str,
    session_id: &mut Option<String>,
    client_addr: SocketAddr,
    sessions: &Arc<RwLock<HashMap<String, RtspSession>>>,
) -> Result<String> {
    let cseq = headers
        .get("CSeq")
        .and_then(|v| v.parse::<u32>().ok())
        .unwrap_or(0);
    
    let response = match method {
        "OPTIONS" => {
            format!(
                "RTSP/1.0 200 OK\r\nCSeq: {}\r\nPublic: DESCRIBE, SETUP, TEARDOWN, PLAY, PAUSE, RECORD\r\n\r\n",
                cseq
            )
        }
        "DESCRIBE" => {
            let sdp = create_sdp(path);
            format!(
                "RTSP/1.0 200 OK\r\nCSeq: {}\r\nContent-Type: application/sdp\r\nContent-Length: {}\r\n\r\n{}",
                cseq,
                sdp.len(),
                sdp
            )
        }
        "SETUP" => {
            // Create or get session
            if session_id.is_none() {
                *session_id = Some(sessions.write().iter().find_map(|(id, s)| {
                    if s.client_addr == client_addr {
                        Some(id.clone())
                    } else {
                        None
                    }
                }).unwrap_or_else(|| {
                    let sid = Uuid::new_v4().to_string();
                    let mut sess = RtspSession::new(client_addr, path.to_string());
                    sess.state = SessionState::SetupComplete;
                    sess.ssrc = Some(rand::random());
                    sessions.write().insert(sid.clone(), sess);
                    metrics::record_rtsp_session_created();
                    sid
                }));
            }
            
            if let Some(ref sid) = session_id {
                let mut sessions_guard = sessions.write();
                if let Some(sess) = sessions_guard.get_mut(sid) {
                    sess.state = SessionState::SetupComplete;
                    sess.transport = headers.get("Transport").cloned();
                    
                    format!(
                        "RTSP/1.0 200 OK\r\nCSeq: {}\r\nSession: {}\r\nTransport: RTP/AVP/TCP;unicast;interleaved=0-1\r\n\r\n",
                        cseq, sid
                    )
                } else {
                    return Err(Error::Rtsp("Session not found".into()));
                }
            } else {
                return Err(Error::Rtsp("Failed to create session".into()));
            }
        }
        "PLAY" => {
            if let Some(ref sid) = session_id {
                let mut sessions_guard = sessions.write();
                if let Some(sess) = sessions_guard.get_mut(sid) {
                    sess.state = SessionState::Playing;
                    format!(
                        "RTSP/1.0 200 OK\r\nCSeq: {}\r\nSession: {}\r\nRange: npt=0-\r\n\r\n",
                        cseq, sid
                    )
                } else {
                    return Err(Error::Rtsp("Session not found".into()));
                }
            } else {
                return Err(Error::Rtsp("No session established".into()));
            }
        }
        "TEARDOWN" => {
            if let Some(ref sid) = session_id {
                let mut sessions_guard = sessions.write();
                if let Some(sess) = sessions_guard.get_mut(sid) {
                    sess.state = SessionState::Terminated;
                }
                sessions_guard.remove(sid);
                *session_id = None;
                metrics::record_rtsp_session_destroyed();
            }
            format!(
                "RTSP/1.0 200 OK\r\nCSeq: {}\r\n\r\n",
                cseq
            )
        }
        "RECORD" => {
            if let Some(ref sid) = session_id {
                let mut sessions_guard = sessions.write();
                if let Some(sess) = sessions_guard.get_mut(sid) {
                    sess.state = SessionState::Recording;
                    format!(
                        "RTSP/1.0 200 OK\r\nCSeq: {}\r\nSession: {}\r\n\r\n",
                        cseq, sid
                    )
                } else {
                    return Err(Error::Rtsp("Session not found".into()));
                }
            } else {
                return Err(Error::Rtsp("No session established".into()));
            }
        }
        _ => {
            format!(
                "RTSP/1.0 501 Not Implemented\r\nCSeq: {}\r\n\r\n",
                cseq
            )
        }
    };
    
    Ok(response)
}

/// Create SDP description for a stream
fn create_sdp(path: &str) -> String {
    let ssrc = rand::random::<u32>();
    format!(
        "v=0\r\n\
         o=- 0 0 IN IP4 127.0.0.1\r\n\
         s={}\r\n\
         c=IN IP4 0.0.0.0\r\n\
         t=0 0\r\n\
         m=video 0 RTP/AVP 96\r\n\
         a=rtpmap:96 H264/90000\r\n\
         a=fmtp:96 packetization-mode=1;profile-level-id=42e01f\r\n\
         a=control:track1\r\n\
         a=ssrc:{} cname:ingest-stream\r\n",
        path, ssrc
    )
}

// Add rand dependency placeholder - will be added to Cargo.toml
mod rand {
    pub fn random<T: Default>() -> T {
        T::default()
    }
}
