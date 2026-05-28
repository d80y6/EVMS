# Media Processing Service

GStreamer-based media processing service for real-time video analytics pipelines.

## Features

- Dynamic GStreamer pipeline creation and management
- Real-time frame preprocessing (resize, normalize)
- Integration with Triton Inference Server
- Post-processing with NMS and confidence filtering
- Prometheus metrics export
- REST API for pipeline control

## Quick Start

```bash
cargo run
```

## API Endpoints

- `GET /health` - Health check
- `GET /pipelines` - List all pipelines
- `POST /pipelines` - Create a new pipeline
- `POST /pipelines/:id/start` - Start a pipeline
- `POST /pipelines/:id/stop` - Stop a pipeline
- `DELETE /pipelines/:id` - Delete a pipeline

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| MEDIA__HTTP_HOST | 0.0.0.0 | HTTP bind address |
| MEDIA__HTTP_PORT | 3004 | HTTP port |
| MEDIA__GST_DEBUG | 2 | GStreamer debug level |
| MEDIA__MAX_PIPELINES | 10 | Maximum concurrent pipelines |
| MEDIA__INFERENCE_ENDPOINT | - | Triton server URL |
