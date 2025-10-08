## Starting the Server

```bash
make server
```

Server runs on http://localhost:8080

Custom port:
```bash
./bin/firecracker-all -action=server -port=3000
```

## API Endpoints

Base URL: `http://localhost:8080/api/v1`

### Packages

```bash
GET    /api/v1/packages
POST   /api/v1/packages/{name}/install
DELETE /api/v1/packages/{name}
```

### Nodes

```bash
GET  /api/v1/nodes
POST /api/v1/nodes/{name}/start
POST /api/v1/nodes/{name}/stop
GET  /api/v1/nodes/{name}/status
GET  /api/v1/nodes/{name}/logs?lines=100
```

### Jobs

```bash
GET /api/v1/jobs/{job_id}
```

### Health

```bash
GET /health
```

## Testing

```bash
curl http://localhost:8080/api/v1/packages
curl -X POST http://localhost:8080/api/v1/packages/ros2-slam/install
curl http://localhost:8080/api/v1/nodes/camera_node/status
curl http://localhost:8080/api/v1/nodes/camera_node/logs
```

## Notes

- All data is fake for development
- Returns proper HTTP status codes
- Server logs all requests
- No authentication required
