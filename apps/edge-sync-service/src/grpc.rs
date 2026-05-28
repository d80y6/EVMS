//! gRPC service implementation for edge sync

use tonic::{Request, Response, Status};
use crate::api::ApiState;
use std::sync::Arc;

// Include generated protobuf code
pub mod sync {
    tonic::include_proto!("edge.sync");
}

use sync::{
    edge_sync_service_server::EdgeSyncService,
    ConflictResolutionChoice, DataEntry as ProtoDataEntry, GetQueueStateRequest,
    GetQueueStateResponse, ResolveConflictRequest, ResolveConflictResponse, SyncRequest,
    SyncResponse, VectorClock as ProtoVectorClock,
};

/// gRPC service implementation
#[derive(Clone)]
pub struct EdgeSyncGrpcService {
    state: Arc<ApiState>,
}

impl EdgeSyncGrpcService {
    pub fn new(state: Arc<ApiState>) -> Self {
        Self { state }
    }

    fn proto_to_data_entry(proto: &ProtoDataEntry) -> crate::DataEntry {
        crate::DataEntry {
            key: proto.key.clone(),
            value: proto.value.clone(),
            content_type: proto.content_type.clone(),
            created_at: proto.created_at,
            updated_at: proto.updated_at,
            metadata: proto.metadata.clone(),
            deleted: proto.deleted,
            version: 0,
            device_id: String::new(),
        }
    }

    fn data_entry_to_proto(entry: &crate::DataEntry) -> ProtoDataEntry {
        ProtoDataEntry {
            key: entry.key.clone(),
            value: entry.value.clone(),
            content_type: entry.content_type.clone(),
            created_at: entry.created_at,
            updated_at: entry.updated_at,
            metadata: entry.metadata.clone(),
            deleted: entry.deleted,
        }
    }

    fn proto_to_vector_clock(proto: &ProtoVectorClock) -> crate::VectorClock {
        crate::VectorClock::from_map(proto.clocks.clone())
    }

    fn vector_clock_to_proto(clock: &crate::VectorClock) -> ProtoVectorClock {
        ProtoVectorClock {
            clocks: clock.to_map(),
        }
    }
}

#[tonic::async_trait]
impl EdgeSyncService for EdgeSyncGrpcService {
    async fn sync(
        &self,
        request: Request<SyncRequest>,
    ) -> Result<Response<SyncResponse>, Status> {
        let req = request.into_inner();
        
        // Process sync request
        let mut entries = Vec::new();
        for proto_entry in &req.entries {
            entries.push(Self::proto_to_data_entry(proto_entry));
        }

        // Store entries
        for entry in entries {
            if let Err(e) = self.state.app_state.storage.put(entry).await {
                return Ok(Response::new(SyncResponse {
                    success: false,
                    entries: vec![],
                    server_clock: None,
                    conflicts: vec![],
                    error_message: e.to_string(),
                }));
            }
        }

        // Get current vector clock
        let clock = self.state.app_state.sync_engine.get_clock().await;

        Ok(Response::new(SyncResponse {
            success: true,
            entries: vec![],
            server_clock: Some(Self::vector_clock_to_proto(&clock)),
            conflicts: vec![],
            error_message: String::new(),
        }))
    }

    type SyncStreamStream = tokio_stream::wrappers::ReceiverStream<Result<sync::SyncMessage, Status>>;

    async fn sync_stream(
        &self,
        request: Request<tonic::Streaming<sync::SyncMessage>>,
    ) -> Result<Response<Self::SyncStreamStream>, Status> {
        let mut stream = request.into_inner();
        let (tx, rx) = tokio::sync::mpsc::channel(32);

        let storage = self.state.app_state.storage.clone();
        
        tokio::spawn(async move {
            while let Some(result) = stream.recv().await {
                match result {
                    Ok(msg) => {
                        // Process message
                        if let Some(payload) = msg.payload {
                            let entry = crate::DataEntry {
                                key: msg.id.clone(),
                                value: payload,
                                content_type: "application/octet-stream".to_string(),
                                created_at: msg.timestamp,
                                updated_at: msg.timestamp,
                                metadata: std::collections::HashMap::new(),
                                deleted: false,
                                version: 0,
                                device_id: msg.device_id.clone(),
                            };

                            let _ = storage.put(entry).await;
                        }

                        // Echo back acknowledgment
                        let response = sync::SyncMessage {
                            id: msg.id,
                            r#type: 1, // DATA
                            timestamp: msg.timestamp,
                            payload: vec![],
                            vector_clock: msg.vector_clock,
                            device_id: msg.device_id,
                        };

                        let _ = tx.send(Ok(response)).await;
                    }
                    Err(e) => {
                        let _ = tx.send(Err(Status::internal(e.to_string()))).await;
                        break;
                    }
                }
            }
        });

        Ok(Response::new(tokio_stream::wrappers::ReceiverStream::new(rx)))
    }

    async fn get_queue_state(
        &self,
        request: Request<GetQueueStateRequest>,
    ) -> Result<Response<GetQueueStateResponse>, Status> {
        let _req = request.into_inner();
        
        let stats = self.state.app_state.offline_queue.stats().await;
        
        let pending_items: Vec<_> = self.state.app_state.offline_queue.get_pending().await
            .into_iter()
            .map(|item| sync::QueueItem {
                id: item.id,
                message: None,
                created_at: item.created_at,
                retry_count: item.retry_count,
                status: match item.status {
                    crate::QueueItemStatus::Pending => 1,
                    crate::QueueItemStatus::InProgress => 2,
                    crate::QueueItemStatus::Completed => 3,
                    crate::QueueItemStatus::Failed => 4,
                },
            })
            .collect();

        Ok(Response::new(GetQueueStateResponse {
            pending_count: stats.pending as u64,
            failed_count: stats.failed as u64,
            pending_items,
        }))
    }

    async fn resolve_conflict(
        &self,
        request: Request<ResolveConflictRequest>,
    ) -> Result<Response<ResolveConflictResponse>, Status> {
        let req = request.into_inner();
        
        // TODO: Implement actual conflict resolution
        Ok(Response::new(ResolveConflictResponse {
            success: true,
            resolved_entry: None,
        }))
    }
}

/// Create gRPC server
pub fn create_grpc_server(
    state: Arc<ApiState>,
    addr: std::net::SocketAddr,
) -> tonic::transport::Server {
    let service = EdgeSyncGrpcService::new(state);
    
    tonic::transport::Server::builder()
        .add_service(sync::edge_sync_service_server::EdgeSyncServiceServer::new(service))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_proto_conversion() {
        let entry = crate::DataEntry::new("key1".to_string(), b"value1".to_vec(), "device1".to_string());
        let proto = EdgeSyncGrpcService::data_entry_to_proto(&entry);
        let restored = EdgeSyncGrpcService::proto_to_data_entry(&proto);
        
        assert_eq!(restored.key, entry.key);
        assert_eq!(restored.value, entry.value);
    }
}
