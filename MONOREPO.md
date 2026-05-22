# Monorepo Structure

- `api/`: gRPC and OpenAPI definitions.
- `deploy/`: Infrastructure as Code (Docker, K8s, Helm).
- `docs/`: Design documents, ADRs, and research.
- `pkg/`: Shared Go libraries.
- `services/`: Independent microservices.
- `web/`: Frontend React application.

## Development Workflow

1. Modify `.proto` files in `api/proto/`.
2. Generate Go/TS code (via `make generate`).
3. Implement logic in `services/`.
4. Add tests.
5. Deploy via `docker-compose`.
