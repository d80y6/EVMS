# Vault Agent Injector Configuration

To use Vault secrets in a pod, add these annotations:

```yaml
annotations:
  vault.hashicorp.com/agent-inject: "true"
  vault.hashicorp.com/role: "evms-service"
  vault.hashicorp.com/agent-inject-secret-database: "evms/database"
  vault.hashicorp.com/agent-inject-template-database: |
    {{`{{- with secret "evms/database" -}}
    export DB_URL={{ .Data.data.DB_URL }}
    export DB_USER={{ .Data.data.DB_USER }}
    {{- end -}}`}}
```

Requires Vault Agent Injector to be installed in the cluster.
