# Observability Guide

## Cross-Signal Navigation
- **Metrics → Traces**: Exemplars on Prometheus histograms link to Jaeger traces
- **Logs → Traces**: Derived fields in Loki extract trace_id, links to Jaeger
- **Traces → Logs**: Jaeger spans link to Loki logs via service name and timestamp

## Service Dependency Graph
Use Grafana Tempo's service graph or Jaeger's dependency graph:
- Jaeger UI: System → Dependencies
- Requires sufficient trace data to generate the graph

## Dashboards
- EVMS Overview: `deploy/docker/grafana/evms-dashboard.json`
- Streaming Pipeline: `deploy/docker/grafana/streaming-dashboard.json`
- AI Pipeline: `deploy/docker/grafana/ai-dashboard.json`
- Auth Dashboard: `deploy/docker/grafana/auth-dashboard.json`
