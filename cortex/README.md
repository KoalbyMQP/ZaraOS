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

## Authentication

All endpoints except `/auth/pair/*` require signed requests.

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

---

## Key Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/identity` | Device info |
| GET | `/apps` | List apps |
| GET | `/apps/{name}/versions` | App version history |
| GET | `/instances` | List instances |
| POST | `/instances` | Start an instance `{"app":"ros2-nav"}` |
| DELETE | `/instances/{id}` | Stop an instance |
| POST | `/instances/{id}/restart` | Restart |
| POST | `/instances/{id}/update` | Update to latest version |
| GET | `/instances/{id}/logs` | Logs (`?stream=true` for SSE) |
| GET | `/instances/{id}/health` | Health check |
| GET | `/instances/{id}/metrics` | CPU / memory / uptime |
| GET | `/diagnostics` | Run all diagnostic tests |
| GET | `/diagnostics/{test}` | Last result for a test |
| POST | `/diagnostics/{test}/run` | Run a specific test |
| GET | `/ros/nodes` | ROS nodes |
| GET | `/ros/topics` | ROS topics |
| GET | `/ros/graph` | Node graph |
| GET | `/events` | Event log (`?since=<RFC3339>&type=<type>`) |
| GET | `/auth/sessions` | Active sessions |
| POST | `/auth/revoke` | Revoke a session by token prefix |
