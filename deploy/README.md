# DAM VMS Deployment

## Docker Compose
For quick start and development, use the `deploy/docker/docker-compose.yml`.

## Kubernetes
Production deployments should use the Helm charts in `deploy/helm/dam-vms`.

### Prerequisites
- Kubernetes Cluster 1.25+
- Helm 3
- NVIDIA Container Toolkit (for GPU acceleration)
- Shared storage (Ceph, MinIO, or Cloud Storage)
