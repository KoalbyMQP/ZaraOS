# Cortex

A lightweight HTTP control plane for managing robot software. It runs on-device and exposes a signed API for deploying apps, monitoring instances, viewing ROS diagnostics, and streaming logs.

---

## Running

**Docker:**
```sh
docker build -t cortex .
docker run -p 8080:8080 cortex
```

**Local:**
```sh
cd cortex
go run ./cmd/
```

Server listens on `:8080`.

---

## Environment Variables

| Variable | Description | Go Default | Dockerfile |
|----------|-------------|------------|------------|
| `GITHUB_ORG` | GitHub organization for fetching releases | `KoalbyMQP` | `KoalbyMQP` |
| `GITHUB_REPOS` | Comma-separated repos to watch for app releases | *(none)* | `Core` |
| `DOCKERHUB_ORG` | Docker Hub organization for container images | `koalbymqp` | `koalby` |
| `REGISTRY_CACHE_TTL` | Cache duration for GitHub release data | `5m` | — |

---

## Authentication

All endpoints except `/health`, `/auth/pair/*`, and `/shell` (ticket-based) require signed requests. Localhost (`127.0.0.1`) bypasses auth entirely for development.

### Pairing Flow

**1. Start pairing — generates a 6-digit code printed to the server terminal:**
```sh
POST /auth/pair/start
```

**2. Complete pairing — submit the code to get a salt:**
```sh
POST /auth/pair/complete
{ "code": "123456", "label": "my-client" }
```

**3. Derive your token locally (never transmitted):**
```
token = hex(HMAC-SHA256(key=code, message=salt))
```

**4. Sign every request:**
```
timestamp  = time.Now().UTC().Format(RFC3339)
bodyHash   = hex(SHA256(raw_body))   # empty body → hash of ""
message    = timestamp + "\n" + METHOD + "\n" + path + "\n" + bodyHash
signature  = hex(HMAC-SHA256(key=token, message=message))
```

Include on every request:
```
X-Timestamp: <RFC3339>
X-Signature: <hex>
```

> Timestamps must be within ±60 seconds of server time to prevent replay attacks.

---

## API Reference

### Health

#### `GET /health`
Returns server health status. No authentication required.

**Response `200`:**
```json
{ "status": "ok" }
```

---

### Authentication

#### `POST /auth/pair/start`
Initiate device pairing. Generates a 6-digit code printed to the server terminal (never returned in the response). **Public.**

**Response `200`:**
```json
{ "expires_in": 120 }
```

---

#### `POST /auth/pair/complete`
Complete pairing by submitting the 6-digit code. Returns a salt used to derive the signing token client-side. **Public.**

**Request:**
```json
{ "code": "123456", "label": "my-client" }
```

**Response `200`:**
```json
{ "salt": "<hex>", "label": "my-client", "created_at": "<RFC3339>" }
```

**Response `401`:** Invalid or expired code.

---

#### `GET /auth/sessions`
List all active paired sessions.

**Response `200`:**
```json
{
  "sessions": [
    { "token_prefix": "a1b2c3", "label": "my-client", "created_at": "<RFC3339>", "last_seen": "<RFC3339>" }
  ]
}
```

---

#### `POST /auth/revoke`
Revoke a session by its token prefix.

**Request:**
```json
{ "token_prefix": "a1b2c3" }
```

**Response `200`:**
```json
{ "revoked": true }
```

**Response `404`:** Session not found.

---

### Identity

#### `GET /identity`
Get device identification information. Returns seeded defaults — not yet reading from real hardware.

**Response `200`:**
```json
{
  "name": "robot-01",
  "serial": "RPi4-ABC123",
  "location": "warehouse-floor-3",
  "firmware_version": "0.4.2"
}
```

---

#### `PUT /identity/location`
Update the device's location.

**Request:**
```json
{ "location": "warehouse-floor-5" }
```

**Response `200`:**
```json
{ "location": "warehouse-floor-5" }
```

**Response `400`:** Location field missing.

---

#### `GET /ssh`
Get SSH connection details for the device. Currently returns hardcoded values — not yet reading from real network config.

**Response `200`:**
```json
{
  "host": "192.168.1.42",
  "port": 22,
  "user": "robot",
  "hint": "ssh robot@192.168.1.42",
  "name": "robot-01"
}
```

---

### Apps

#### `GET /apps`
List all available apps from the registry (GitHub releases).

**Response `200`:**
```json
{
  "apps": [
    { "name": "ros2-nav", "repo": "Core", "latest_version": "v1.2.3" }
  ]
}
```

---

#### `GET /apps/{name}/versions`
Get version history for a specific app.

**Response `200`:**
```json
{
  "app": "ros2-nav",
  "versions": [
    { "version": "v1.2.3", "published_at": "<RFC3339>", "changelog": "Bug fixes" }
  ]
}
```

**Response `404`:** App not found.

---

### Images (Local Registry)

Endpoints for inspecting and managing container images stored locally on the device. These reflect what has been pulled or built via nerdctl/containerd — not what is available upstream.

> Repository names contain slashes (e.g. `docker.io/koalbymqp/ros2-nav`). In URL path segments, **replace slashes with underscores**: `docker.io_koalbymqp_ros2-nav`.

#### `GET /images`
List all locally available container images, grouped by repository.

**Response `200`:**
```json
{
  "images": [
    {
      "repository": "docker.io/koalbymqp/ros2-nav",
      "tags": ["v1.2.3", "latest"],
      "id": "sha256:abc123...",
      "size": "142.5 MiB",
      "created_at": "2025-06-15 09:30:00 +0000 UTC"
    }
  ]
}
```

---

#### `GET /images/{name}/tags`
List all tags for a specific image repository. Use underscores in place of slashes in `{name}`.

**Example:** `GET /images/docker.io_koalbymqp_ros2-nav/tags`

**Response `200`:**
```json
{
  "repository": "docker.io/koalbymqp/ros2-nav",
  "tags": ["v1.2.3", "v1.2.2", "latest"]
}
```

**Response `404`:** No tags found for the image.

---

#### `DELETE /images/{name}`
Remove a local image. Use underscores in place of slashes in `{name}`.

**Query Parameters:**
| Param | Default | Description |
|-------|---------|-------------|
| `tag` | — | Remove only this tag (e.g. `?tag=v1.2.2`). Omit to remove all tags. |
| `force` | `false` | Force removal even if containers reference the image (`?force=true`). |

**Example:** `DELETE /images/docker.io_koalbymqp_ros2-nav?tag=v1.2.2`

**Response `200`:**
```json
{
  "deleted": "docker.io/koalbymqp/ros2-nav:v1.2.2",
  "detail": "Untagged: docker.io/koalbymqp/ros2-nav:v1.2.2"
}
```

**Response `404`:** Image not found.
**Response `409`:** Image is in use by a running container (use `?force=true` to override).

---

### Instances

#### `GET /instances`
List all container instances.

**Response `200`:**
```json
{
  "instances": [
    {
      "id": "ros2-nav-v1.2.3-1700000000",
      "app": "ros2-nav",
      "version": "v1.2.3",
      "image": "docker.io/koalbymqp/ros2-nav:v1.2.3",
      "state": "running",
      "error": null,
      "started_at": "<RFC3339>",
      "stopped_at": null
    }
  ]
}
```

---

#### `GET /instances/{id}`
Get detailed information for a specific instance. Includes `container_id` and `exit_code` beyond what the list endpoint returns.

**Response `200`:** Full instance object.

**Response `404`:** Instance not found.

---

#### `POST /instances`
Start a new container instance. The `app` field is required. If `version` is omitted or `"latest"`, the newest version from the registry is used. Optionally override the `image`.

**Request:**
```json
{ "app": "ros2-nav", "version": "latest", "image": "<optional-override>" }
```

**Response `201`:** Instance summary. Container is started synchronously via nerdctl — response reflects the final state (`"running"` on success, `"crashed"` on failure).

**Response `400`:** Missing `app` field.
**Response `404`:** App or version not found.
**Response `409`:** Instance already running for this app.

---

#### `DELETE /instances/{id}`
Stop a running container instance.

**Response `200`:**
```json
{ "id": "ros2-nav-v1.2.3-1700000000", "state": "stopped" }
```

**Response `404`:** Instance not found.

---

#### `POST /instances/{id}/restart`
Restart an instance (stops old, starts new with same version).

**Response `201`:**
```json
{
  "old_id": "...",
  "new_id": "...",
  "app": "ros2-nav",
  "version": "v1.2.3",
  "state": "starting",
  "started_at": "<RFC3339>"
}
```

**Response `404`:** Instance not found.

---

#### `POST /instances/{id}/update`
Update an instance to the latest available version. If already on the latest, returns `200` with no changes.

**Response `201`:**
```json
{
  "old_id": "...",
  "old_version": "v1.2.2",
  "new_id": "...",
  "new_version": "v1.2.3",
  "state": "starting",
  "started_at": "<RFC3339>"
}
```

**Response `404`:** Instance not found.

---

#### `GET /instances/{id}/logs`
Fetch container logs. Supports one-shot and streaming modes.

**Query Parameters:**
| Param | Default | Description |
|-------|---------|-------------|
| `tail` | `100` | Number of log lines to return |
| `stream` | `false` | Set to `true` for Server-Sent Events streaming |

**Response `200` (non-streaming):** Plain text log output.

**Response `200` (streaming):** SSE stream with `data: <log-line>` events. Stays open until the client disconnects.

**Response `404`:** Instance not found.

---

#### `GET /instances/{id}/health`
Health check for a specific instance. Checks if the container is running via nerdctl.

**Response `200`:**
```json
{
  "id": "...",
  "healthy": true,
  "last_checked": "<RFC3339>",
  "output": "OK"
}
```

**Response `404`:** Instance not found.

---

#### `GET /instances/{id}/metrics`
Get CPU, memory, and uptime metrics for an instance via nerdctl stats.

**Response `200`:**
```json
{
  "id": "...",
  "cpu_percent": 0.42,
  "memory_mb": 84.5,
  "uptime_seconds": 3600
}
```

**Response `404`:** Instance not found.

---

### Diagnostics

> Diagnostic tests currently return seeded/mock results — not yet running real hardware checks.

#### `GET /diagnostics`
Run all diagnostic tests and return aggregated results.

**Response `200`:**
```json
{
  "ran_at": "<RFC3339>",
  "passed": 3,
  "failed": 1,
  "results": [
    { "name": "lidar-ping", "passed": true, "output": "LIDAR responding at 10Hz", "duration_ms": 320, "ran_at": "<RFC3339>" },
    { "name": "camera-check", "passed": false, "output": "No device found at /dev/video0", "duration_ms": 50, "ran_at": "<RFC3339>" }
  ]
}
```

**Built-in tests:**
| Test | Description | Duration |
|------|-------------|----------|
| `lidar-ping` | LIDAR sensor responsiveness | ~320ms |
| `camera-check` | Camera device detection | ~50ms |
| `network-reachability` | Gateway connectivity | ~80ms |
| `disk-space` | Disk usage check | ~10ms |

---

#### `GET /diagnostics/history`
Get historical diagnostic run summaries. Currently returns generated mock data (max 5 entries) — not yet persisting real run history.

**Query Parameters:**
| Param | Default | Description |
|-------|---------|-------------|
| `limit` | `20` | Max number of history entries (capped at 5 with mock data) |

**Response `200`:**
```json
{
  "history": [
    { "ran_at": "<RFC3339>", "passed": 4, "failed": 0 }
  ]
}
```

---

#### `GET /diagnostics/{test}`
Get the last result for a specific diagnostic test.

**Response `200`:**
```json
{ "name": "lidar-ping", "passed": true, "output": "LIDAR responding at 10Hz", "duration_ms": 320, "ran_at": "<RFC3339>" }
```

**Response `404`:** Test not found.

---

#### `POST /diagnostics/{test}/run`
Run a specific diagnostic test on demand.

**Response `200`:**
```json
{ "name": "lidar-ping", "passed": true, "output": "LIDAR responding at 10Hz", "duration_ms": 320, "ran_at": "<RFC3339>" }
```

**Response `404`:** Test not found.

---

### ROS Integration

> ROS endpoints currently return mock/seeded data.

#### `GET /ros/nodes`
List active ROS 2 nodes.

**Response `200`:**
```json
{
  "nodes": [
    { "name": "/navigation", "namespace": "/", "status": "active" }
  ]
}
```

---

#### `GET /ros/topics`
List ROS 2 topics with type and subscriber info.

**Response `200`:**
```json
{
  "topics": [
    { "name": "/cmd_vel", "type": "geometry_msgs/msg/Twist", "publishers": 1, "subscribers": 2 }
  ]
}
```

---

#### `GET /ros/topic/{name}/echo`
Echo recent messages from a ROS topic. Use underscores in place of slashes for nested topic names.

**Response `200`:**
```json
{ "topic": "/cmd_vel", "type": "geometry_msgs/msg/Twist", "messages": [] }
```

---

#### `GET /ros/services`
List available ROS 2 services.

**Response `200`:**
```json
{
  "services": [
    { "name": "/navigation/reset", "type": "std_srvs/srv/Trigger" }
  ]
}
```

---

#### `GET /ros/graph`
Get the ROS node-topic graph showing connections between nodes.

**Response `200`:**
```json
{
  "nodes": ["..."],
  "edges": [
    { "from": "/lidar_driver", "to": "/navigation", "topic": "/scan" }
  ]
}
```

---

### Events

#### `GET /events`
Query the event log with optional filtering. Events are retained in-memory (last 500).

**Query Parameters:**
| Param | Default | Description |
|-------|---------|-------------|
| `since` | — | RFC3339 timestamp; return events after this time |
| `type` | — | Filter by event type |
| `limit` | `50` | Max events to return |

**Response `200`:**
```json
{
  "events": [
    { "id": "evt_0001", "type": "instance_started", "timestamp": "<RFC3339>", "data": { "app": "ros2-nav" } }
  ]
}
```

**Event types:** `auth_paired`, `auth_revoked`, `instance_started`, `instance_start_failed`, `instance_stopped`, `image_deleted`, `diagnostic_run`

---

### Shell (WebSocket)

#### `POST /shell/ticket`
Issue a one-time ticket for WebSocket shell access. Ticket expires after 30 seconds.

**Response `200`:**
```json
{ "ticket": "<base64-token>", "expires_in": 30 }
```

---

#### `GET /shell?ticket=<ticket>`
Upgrade to a WebSocket connection for an interactive shell session. **Public** (authenticated via ticket). Spawns `/bin/sh` with a full PTY.

**WebSocket Protocol:**
- **Binary frames (server → client):** PTY output
- **Binary frames (client → server):** Keyboard input (written to PTY)
- **Text frames (client → server):** Control messages only — `{"type": "resize", "cols": 120, "rows": 40}`

Connection stays open until the client disconnects or the shell process exits.

---

## Data Models

### Instance
```
id             string    — Unique ID (app-version-timestamp)
app            string    — App name
version        string    — Semantic version
image          string    — Full container image reference
container_id   string    — nerdctl container name
state          string    — starting | running | stopping | stopped | crashed
error          string    — Failure message (if any)
started_at     time      — When the instance was created
stopped_at     time?     — When the instance was stopped
pid            int?      — OS process ID
exit_code      int?      — Process exit code
```

### Event
```
id             string           — Sequential event ID (evt_XXXX)
type           string           — Event type name
timestamp      time             — When the event occurred
data           map[string]any   — Arbitrary event payload
```

### DiagResult
```
name           string    — Test name
passed         bool      — Whether the test passed
output         string    — Human-readable output
duration_ms    int       — Execution time in milliseconds
ran_at         time      — When the test was run
```

### Identity
```
name              string — Device name
serial            string — Hardware serial number
location          string — Physical location
firmware_version  string — Current firmware version
```

### LocalImage
```
repository     string     — Image repository name (e.g. docker.io/koalbymqp/ros2-nav)
tags           []string   — All tags present locally
id             string     — Image digest / ID
size           string     — Human-readable image size
created_at     string     — When the image was created
```

### App (Registry)
```
name           string    — App name
repo           string    — Source GitHub repository
versions       []Version — Version history (newest first)
```

### Version
```
version        string — Semantic version tag
published_at   time   — Release date
changelog      string — Release notes
```

---

## Architecture Notes

- **Thread-safe store** — All data structures protected by `sync.RWMutex`
- **Synchronous container ops** — Instance start/stop/restart/update run nerdctl synchronously; errors are captured in `instance.Error`
- **SSE streaming** — Log streaming uses HTTP flusher with context cancellation
- **PTY shell** — Full interactive terminal via WebSocket + `creack/pty`
- **Local image registry** — `GET /images` queries nerdctl directly for on-device images; no caching layer (always live)
- **Registry caching** — GitHub releases cached with configurable TTL; falls back to stale cache on fetch errors
- **Event retention** — Last 500 events kept in a rotating in-memory buffer
- **Graceful shutdown** — 10-second timeout for connection draining
- **CORS** — Enabled on all routes with preflight handling
