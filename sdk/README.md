# EVMS SDK

Go client library for the EVMS (Enterprise Video Management System) API.

## Usage

```go
import "github.com/dam-vms/dam/sdk/evms"

client := evms.NewClient("http://localhost:8090", "your-api-key")

cameras, err := client.ListCameras()
events, err := client.GetEvents("camera-id", time.Now().Add(-1*time.Hour))
err := client.SendPTZCommand("camera-id", "left")
recordings, err := client.SearchRecordings("camera-id", startTime, endTime)
```

## Plugin Interface

Plugins integrate via:

1. **REST API** - Register as a plugin endpoint, receive forwarded events via POST
2. **NATS** - Subscribe to event subjects directly using the EVMS NATS cluster

### Registering a Plugin

```
POST /api/plugins
{
  "name": "my-plugin",
  "version": "1.0.0",
  "description": "Custom analytics plugin",
  "endpoint": "http://my-plugin:9000/events",
  "permissions": ["events:read"]
}
```

Plugins receive event data as JSON POSTs to their registered endpoint whenever matching events occur in the system.
