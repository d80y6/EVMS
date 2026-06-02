# EVMS Operations Runbook

## Service Health

Check overall service status:
```bash
kubectl get pods -n dam-vms
kubectl get deployments -n dam-vms
```

Check individual service health endpoints:
```bash
kubectl port-forward -n dam-vms svc/api-gateway 8090:8090
curl http://localhost:8090/health
```

## Common Incidents

### Service CrashLoopBackOff
1. Check logs: `kubectl logs -n dam-vms deployment/<service> --tail=100`
2. Check events: `kubectl describe pod -n dam-vms <pod-name>`
3. Common causes: DB connection failure, NATS unavailable, missing secrets
4. Verify config: `kubectl get secret -n dam-vms evms-secrets -o yaml`
5. Restart: `kubectl rollout restart -n dam-vms deployment/<service>`

### Database Connection Issues
1. Check if DB is running: `kubectl get pods -n dam-vms | grep db`
2. Test connectivity: `kubectl exec -n dam-vms deploy/auth-service -- pg_isready -h db`
3. Check connection pool: `curl http://localhost:2112/metrics | grep vms_db_connections`
4. Verify DB URL secret: `kubectl get secret -n dam-vms evms-secrets -o jsonpath='{.data.db-url}' | base64 -d`

### Camera Stream Down
1. Check alert details for `camera_id`
2. Verify camera network connectivity
3. Check ingest service logs: `kubectl logs -n dam-vms deployment/ingest --tail=50`
4. Restart ingest: `kubectl rollout restart -n dam-vms deployment/ingest`

### Recording Issues
1. Check recorder logs: `kubectl logs -n dam-vms deployment/recorder-service --tail=50`
2. Verify disk space: `curl http://recorder-service:2112/metrics | grep vms_disk_free`
3. Check NATS JetStream: `kubectl exec -n dam-vms deploy/nats-0 -- nats stream list`
4. Restart recorder if stuck: `kubectl rollout restart -n dam-vms deployment/recorder-service`

### NATS Cluster Issues
1. Check NATS pods: `kubectl get pods -n dam-vms -l app=nats`
2. Check NATS routes: `kubectl exec -n dam-vms deploy/nats-0 -- nats server check connection`
3. Verify JetStream: `kubectl exec -n dam-vms deploy/nats-0 -- nats stream report`
4. Restart failed NATS pod: `kubectl delete pod -n dam-vms nats-<id>`

## Recovery Procedures

### Full Database Restore
```bash
# List available backups
kubectl exec -n dam-vms deploy/db-backup -- ls /backups/

# Trigger restore job
kubectl create job -n dam-vms --from=cronjob/db-backup manual-restore

# Or restore specific backup
kubectl apply -f deploy/k8s/restore-job.yaml
```

### Point-in-Time Recovery (PITR)
```bash
# Find target time
kubectl logs -n dam-vms deployment/recorder-service --since=24h | grep "event"

# Run PITR
kubectl exec -n dam-vms deploy/db-backup -- /scripts/pitr_restore.sh "2026-06-01 14:30:00 UTC"
```

### Rolling Back a Deployment
```bash
# Check revision history
kubectl rollout history -n dam-vms deployment/<service>

# Rollback to previous revision
kubectl rollout undo -n dam-vms deployment/<service>

# Rollback to specific revision
kubectl rollout undo -n dam-vms deployment/<service> --to-revision=<N>

# Verify rollback
kubectl rollout status -n dam-vms deployment/<service>
```

### Migration Rollback
```bash
# Connect to DB pod
kubectl exec -n dam-vms deploy/db -- psql -U dam_admin -d dam_vms

# Check applied migrations
SELECT version, applied_at FROM schema_migrations ORDER BY applied_at DESC;

# Rollback last migration (requires down.sql file)
# Re-run the service with ROLLBACK_MIGRATIONS env var
kubectl set env -n dam-vms deployment/<service> ROLLBACK_MIGRATIONS=1
kubectl rollout restart -n dam-vms deployment/<service>
```

## Maintenance

### Draining a Node
```bash
kubectl drain <node-name> --ignore-daemonsets --delete-emptydir-data
kubectl uncordon <node-name>
```

### Upgrading a Service
```bash
# Update image tag
kubectl set image -n dam-vms deployment/<service> <container>=damvms/<service>:<new-tag>

# Monitor rollout
kubectl rollout status -n dam-vms deployment/<service>
```

### Certificate Rotation
```bash
# Regenerate certs (if cert-manager enabled)
kubectl delete secret -n dam-vms <cert-secret>
# Cert-manager will auto-renew

# Manual TLS cert update
kubectl create secret tls -n dam-vms evms-tls --cert=new.crt --key=new.key --dry-run=client -o yaml | kubectl apply -f -
```

## Alert Response

### Critical Alerts
- **ServiceDown**: Check pod status and logs. Restart if needed.
- **StreamInactive**: Check camera network and ingest service.
- **CircuitBreakerOpen**: Downstream service may be unhealthy. Check dependencies.
- **DiskSpaceLow**: Free space or expand PVC.

### Warning Alerts
- **HighMemoryUsage**: Consider increasing resource limits.
- **LowFrameProcessingRate**: Check AI worker and ingest pipeline.
- **NoRecordingsIndexed**: Check recorder and DB connectivity.
- **BackupStale**: Trigger manual backup or check cronjob status.

## Health Endpoints
All services expose:
- `GET /health` - Liveness probe
- `GET /ready` - Readiness probe (fails if dependencies are down)
- `GET /metrics` - Prometheus metrics on port 2112

## Useful Commands
```bash
# Get all service logs (last 1 hour)
kubectl logs -n dam-vms --since=1h deployment/<service>

# Port forward for local testing
kubectl port-forward -n dam-vms svc/api-gateway 8090:8090

# Exec into service pod
kubectl exec -n dam-vms deployment/<service> -it -- sh

# Check resource usage
kubectl top pods -n dam-vms

# Get Prometheus targets
curl http://localhost:9090/api/v1/targets
```
