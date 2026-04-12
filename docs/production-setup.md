# Chat Service — Production Setup Guide

Full architecture reference and step-by-step guide for deploying `be-modami-chat-service` to production.

---

## Table of Contents

1. [System Architecture](#1-system-architecture)
2. [Infrastructure Dependencies](#2-infrastructure-dependencies)
3. [Service Components](#3-service-components)
4. [Full Request Flow](#4-full-request-flow)
5. [Centrifugo Configuration](#5-centrifugo-configuration)
6. [Kafka Topics](#6-kafka-topics)
7. [ScyllaDB Schema](#7-scylladb-schema)
8. [Configuration Reference](#8-configuration-reference)
9. [Docker / Docker Compose](#9-docker--docker-compose)
10. [Kubernetes Deployment](#10-kubernetes-deployment)
11. [Environment Variables & Secrets](#11-environment-variables--secrets)
12. [Local Development](#12-local-development)
13. [Critical Gotchas](#13-critical-gotchas)

---

## 1. System Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Clients (Web / Mobile)                   │
└────────────┬──────────────────────────────────┬─────────────────┘
             │ REST / HTTPS                      │ WebSocket (WSS)
             ▼                                  ▼
┌────────────────────┐              ┌────────────────────────┐
│   API Gateway /    │              │      Centrifugo v5     │
│   Load Balancer    │              │  (shared with noti-svc)│
└────────┬───────────┘              └───────┬────────────────┘
         │                                  │ proxy callbacks
         ▼                    ┌─────────────┴──────────────┐
┌─────────────────┐           ▼                            ▼
│  chat-service   │◄──  /centrifugo/proxy/        /centrifugo/proxy/
│  :8090          │     subscribe                  publish
│                 │
│  ┌───────────┐  │     ┌──────────────────────────────────────────┐
│  │ HTTP API  │  │     │ noti-service handles /connect proxy      │
│  │ Handlers  │  │     │ (JWT validation only — no chat logic)    │
│  └─────┬─────┘  │     └──────────────────────────────────────────┘
│        │        │
│  ┌─────▼─────┐  │
│  │  Service  │  │
│  │  Layer    │  │
│  └─────┬─────┘  │
│        │        │
│  ┌─────▼────────────────────────────────────────┐ │
│  │          Adapters                             │ │
│  │  ScyllaDB  │  Redis  │  Kafka  │  Centrifugo │ │
│  └────────────────────────────────────────────── ┘ │
└─────────────────┘
         │              │              │              │
         ▼              ▼              ▼              ▼
   ScyllaDB 6.x     Redis 7       Kafka 3.x      Centrifugo v5
  (messages,        (cache,      (events:         (realtime
   conversations)    presence,   sent/updated/     broadcast)
                     unread,     deleted/
                     rate limit) reactions)
```

### Key Design Decisions

| Decision | Choice | Reason |
|----------|--------|--------|
| Message store | ScyllaDB | Partition by `conversation_id`, wide rows for history |
| Realtime | Centrifugo v5 | Shared WS server with noti-service (1 connection per client) |
| Event bus | Kafka | Async fan-out to other services (push notifications, search indexing) |
| Cache | Redis | Unread counts, presence, rate limiting |
| Auth | Keycloak JWT (JWKS) | Same token used for REST + WebSocket |

---

## 2. Infrastructure Dependencies

| Service | Version | Purpose | Port |
|---------|---------|---------|------|
| ScyllaDB | 6.2+ | Primary message/conversation store | 9042 |
| Redis | 7+ | Cache, presence, rate limiting | 6379 |
| Kafka | 3.x | Event bus for async downstream consumers | 9092 |
| Centrifugo | v5 | WebSocket server (shared with noti-service) | 8000 (HTTP), 8001 (WS) |
| Keycloak | 24+ | JWT issuer / JWKS endpoint | 8180 (dev) / 443 (prod) |

---

## 3. Service Components

### HTTP API (`cmd/server`)

Exposes REST endpoints on `:8090` (configurable):

| Route | Method | Auth | Description |
|-------|--------|------|-------------|
| `/health` | GET | None | Health check |
| `/swagger/*` | GET | None | Swagger UI |
| `/centrifugo/proxy/subscribe` | POST | None (internal) | Centrifugo subscribe proxy |
| `/centrifugo/proxy/publish` | POST | None (internal) | Centrifugo publish proxy |
| `/v1/chat-services/conversations` | GET | JWT | List user conversations |
| `/v1/chat-services/conversations/direct` | POST | JWT | Create direct conversation |
| `/v1/chat-services/conversations/group` | POST | JWT | Create group conversation |
| `/v1/chat-services/conversations/{id}` | GET | JWT | Get conversation details |
| `/v1/chat-services/conversations/{id}/messages` | GET | JWT | List messages (paginated) |
| `/v1/chat-services/conversations/{id}/messages` | POST | JWT | Send message |
| `/v1/chat-services/conversations/{id}/messages/{msgId}` | PUT | JWT | Edit message |
| `/v1/chat-services/conversations/{id}/messages/{msgId}` | DELETE | JWT | Delete message |
| `/v1/chat-services/conversations/{id}/messages/{msgId}/reactions` | POST | JWT | Add reaction |
| `/v1/chat-services/conversations/{id}/messages/{msgId}/reactions/{emoji}` | DELETE | JWT | Remove reaction |
| `/v1/chat-services/conversations/{id}/read` | POST | JWT | Mark as read |
| `/v1/chat-services/conversations/{id}/typing` | POST | JWT | Typing indicator |

### Kafka Consumer

Subscribes to `chat.messages.inbound` for messages originating from other services.

### Background Workers

- **Presence cleanup** — runs every 30s to remove stale presence entries from Redis.

---

## 4. Full Request Flow

### REST: Send Message

```
Client
  │ POST /v1/chat-services/conversations/{id}/messages
  │ Authorization: Bearer <keycloak-jwt>
  ▼
chat-service HTTP handler
  │ 1. Validate JWT (Keycloak JWKS)
  │ 2. Rate limit check (Redis)
  │ 3. Load conversation from ScyllaDB
  │ 4. Verify sender is a member
  │ 5. Persist message to ScyllaDB (chat.messages + chat.messages_by_id)
  │ 6. Update conversation.last_message (ScyllaDB)
  │ 7. Publish event → Kafka (chat.messages.sent)
  │ 8. Publish realtime event → Centrifugo API (chat:room:{convId})
  │ 9. Increment unread counters → Redis (other participants)
  ▼
Client ← 200 { message }
  │
  │ Centrifugo fans out to all subscribed clients:
  ▼
Other clients receive WS push:
  { "type": "message.new", "data": { ...message } }
```

### WebSocket: Subscribe to Chat Room

```
Client
  │ WS connect to Centrifugo with token (issued by noti-service)
  ▼
Centrifugo → POST noti-service /centrifugo/connect
  ← { user: "user-id", channels: ["noti:user:user-id"] }

Client → subscribe "chat:room:{conversationId}"
  ▼
Centrifugo → POST chat-service /centrifugo/proxy/subscribe
  │ chat-service checks: is user a member of conversationId?
  ├── YES → { result: {} }   (allow)
  └── NO  → { error: { code: 403 } }   (deny)
```

### WebSocket: Client-initiated Publish

```
Client → publish to "chat:room:{conversationId}" via WS
  ▼
Centrifugo → POST chat-service /centrifugo/proxy/publish
  │ chat-service:
  │   1. Validates channel prefix "chat:room:"
  │   2. Calls chatSvc.SendMessage (same flow as REST)
  └─ { result: {} }
```

---

## 5. Centrifugo Configuration

Centrifugo is **shared with noti-service**. One instance, two namespaces.

### Production Config (`deploy/centrifugo/config.json`)

```json
{
  "token_hmac_secret_key": "${CENTRIFUGO_HMAC_SECRET}",
  "api_key": "${CENTRIFUGO_API_KEY}",
  "admin": true,
  "admin_password": "${CENTRIFUGO_ADMIN_PASSWORD}",
  "admin_secret": "${CENTRIFUGO_ADMIN_SECRET}",

  "engine": "redis",
  "redis_address": "${REDIS_HOST}:6379",
  "redis_password": "${REDIS_PASSWORD}",

  "allowed_origins": ["https://app.modami.com"],
  "client_channel_limit": 128,
  "log_level": "info",

  "proxy_connect_endpoint": "http://noti-service:7072/centrifugo/connect",
  "proxy_connect_timeout": "5s",

  "namespaces": [
    {
      "name": "noti",
      "presence": false,
      "history_size": 20,
      "history_ttl": "7d",
      "force_recovery": true,
      "proxy_subscribe": true,
      "proxy_subscribe_endpoint": "http://noti-service:7072/centrifugo/subscribe",
      "proxy_subscribe_timeout": "3s",
      "proxy_publish": true,
      "proxy_publish_endpoint": "http://noti-service:7072/centrifugo/publish",
      "proxy_publish_timeout": "3s"
    },
    {
      "name": "chat",
      "presence": true,
      "join_leave": true,
      "history_size": 100,
      "history_ttl": "24h",
      "force_recovery": true,
      "proxy_subscribe": true,
      "proxy_subscribe_endpoint": "http://chat-service:8090/centrifugo/proxy/subscribe",
      "proxy_subscribe_timeout": "5s",
      "proxy_publish": true,
      "proxy_publish_endpoint": "http://chat-service:8090/centrifugo/proxy/publish",
      "proxy_publish_timeout": "5s"
    }
  ]
}
```

### Local Dev Config (`deploy/centrifugo/config.local.json`)

```json
{
  "token_hmac_secret_key": "${CENTRIFUGO_HMAC_SECRET}",
  "api_key": "${CENTRIFUGO_API_KEY}",
  "admin": true,
  "admin_password": "${CENTRIFUGO_ADMIN_PASSWORD}",
  "admin_secret": "${CENTRIFUGO_ADMIN_SECRET}",
  "allowed_origins": ["http://localhost:3000", "http://localhost:5173"],
  "client_channel_limit": 128,
  "log_level": "debug",
  "proxy_subscribe_endpoint": "http://HOST_IP:8090/centrifugo/proxy/subscribe",
  "proxy_subscribe_timeout": "5s",
  "proxy_publish_endpoint": "http://HOST_IP:8090/centrifugo/proxy/publish",
  "proxy_publish_timeout": "5s",
  "namespaces": [
    {
      "name": "chat",
      "presence": true,
      "join_leave": true,
      "history_size": 100,
      "history_ttl": "24h",
      "force_recovery": true,
      "proxy_subscribe": true,
      "proxy_publish": true
    }
  ]
}
```

> **`HOST_IP`** — When Centrifugo runs in Docker and chat-service runs on the host, `host.docker.internal` may not be routable on all platforms. Find the actual host IP from inside the container:
> ```bash
> docker run --rm alpine ip route show | awk '/default/ {print $3}'
> # or on macOS:
> docker run --rm alpine wget -q -O- http://host.docker.internal:8090/health
> ```
> Use that IP directly in the config.

### Centrifugo v5 Proxy Config Rules

| Centrifugo v4 (old) | Centrifugo v5 (correct) |
|---------------------|------------------------|
| `"proxies": [{"name": "x", "endpoint": "..."}]` | Top-level `"proxy_subscribe_endpoint": "..."` |
| namespace: `"subscribe_proxy_name": "x"` | namespace: `"proxy_subscribe": true` |
| namespace: `"subscribe_proxy_enabled": true` | namespace: `"proxy_subscribe": true` |

In v5, per-namespace proxy endpoints override the top-level ones. The `proxies` array is **not supported**.

### Environment Variables for Centrifugo v5

| Env Var | Maps to config key |
|---------|-------------------|
| `CENTRIFUGO_TOKEN_HMAC_SECRET_KEY` | `token_hmac_secret_key` |
| `CENTRIFUGO_API_KEY` | `api_key` |
| `CENTRIFUGO_ADMIN_PASSWORD` | `admin_password` |
| `CENTRIFUGO_ADMIN_SECRET` | `admin_secret` |

> ⚠️ `CENTRIFUGO_HMAC_SECRET` does **not** map to `token_hmac_secret_key`. Use `CENTRIFUGO_TOKEN_HMAC_SECRET_KEY`.

---

## 6. Kafka Topics

### Topic Naming

The `internal/adapter/messaging/producer.go` uses `EmitToFullTopic` with bare topic names (no env prefix). Topic names must match exactly.

| Topic | Direction | Purpose |
|-------|-----------|---------|
| `chat.messages.sent` | Produced | New message published |
| `chat.messages.updated` | Produced | Message edited |
| `chat.messages.deleted` | Produced | Message deleted |
| `chat.reactions` | Produced | Reaction added/removed |
| `chat.read_receipts` | Produced | Read receipt acknowledged |
| `chat.messages.inbound` | Consumed | Messages from other services |

> **Note on env prefix:** The Kafka consumer path uses `GetTopicWithEnv(env, topic)` (applies `{env}.` prefix from config `kafka.env`). The producer uses `EmitToFullTopic` (no prefix). For consistency, **leave `kafka.env` empty in production** and create topics with bare names as above, OR ensure the env prefix is also applied in `producer.go`.

### Create Topics (Production / Manual Bootstrap)

```bash
# Adjust --bootstrap-server, --replication-factor as needed
for topic in \
  chat.messages.sent \
  chat.messages.updated \
  chat.messages.deleted \
  chat.reactions \
  chat.read_receipts \
  chat.messages.inbound; do
  kafka-topics.sh \
    --bootstrap-server kafka:9092 \
    --create --if-not-exists \
    --topic "$topic" \
    --partitions 6 \
    --replication-factor 3
done
```

### Create Topics (Docker / Local)

```bash
for topic in chat.messages.sent chat.messages.updated chat.messages.deleted chat.reactions chat.read_receipts chat.messages.inbound; do
  docker exec broker kafka-topics \
    --bootstrap-server localhost:9092 \
    --create --if-not-exists \
    --topic "$topic" --partitions 1 --replication-factor 1
done
```

---

## 7. ScyllaDB Schema

Schema is auto-applied at startup via `EnsureSchema` in `internal/adapter/repository/schema.go`.

### Tables

| Table | Key | Purpose |
|-------|-----|---------|
| `chat.messages` | `(conversation_id), created_at DESC, id` | Message store, clustered newest-first |
| `chat.messages_by_id` | `id` | Lookup table: `id → (conversation_id, created_at)` |
| `chat.conversations` | `id` | Conversation metadata |
| `chat.user_conversations` | `(user_id), conversation_id` | Forward index: user → their conversations |
| `chat.conversation_participants` | `(conversation_id), user_id` | Reverse index: conversation → members |
| `chat.direct_conversations` | `lookup_key` | Dedup: sorted pair key → conversation_id |

### Critical: Tablets Must Be Disabled

ScyllaDB 6.x enables "tablets" by default. Tablets do **not** support LWT (`IF NOT EXISTS` / `INSERT ... IF NOT EXISTS`). The schema uses LWT for deduplication on `direct_conversations`.

The keyspace DDL must include:
```sql
AND tablets = {'enabled': false}
```

This is already handled in `EnsureSchema` but matters when creating keyspaces manually:

```sql
-- Single-node (dev/staging)
CREATE KEYSPACE IF NOT EXISTS chat
  WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1}
  AND durable_writes = true
  AND tablets = {'enabled': false};

-- Multi-DC (production)
CREATE KEYSPACE IF NOT EXISTS chat
  WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3}
  AND durable_writes = true
  AND tablets = {'enabled': false};
```

### Drop and Recreate (dev/staging only)

```bash
docker exec -i scylladb cqlsh -e "DROP KEYSPACE IF EXISTS chat;"
# Restart chat-service → EnsureSchema recreates it
```

---

## 8. Configuration Reference

Config is loaded from YAML. Path defaults to `config/config.yaml`, overridden by `CHAT_CONFIG_PATH` env var.

```yaml
app:
  name: "be-modami-chat-service"
  version: "1.0.0"
  environment: "production"        # local | staging | production
  port: 8090
  host: "0.0.0.0"
  swagger_host: "api.modami.com"   # disable swagger in prod if needed
  shutdown_timeout: "30s"
  read_timeout: "30s"
  write_timeout: "30s"
  idle_timeout: "120s"
  allowed_origins:
    - "https://app.modami.com"
  cors_max_age: 300

scylladb:
  hosts:
    - "scylladb-node1"
    - "scylladb-node2"
    - "scylladb-node3"
  keyspace: "chat"
  num_conns: 4                     # connections per host
  datacenter: "dc1"                # must match ScyllaDB DC name
  replication_factor: 3

redis:
  host: "redis"
  port: 6379
  database: 0
  rate_limit_database: 5
  pass: "${REDIS_PASSWORD}"
  pool_size: 100
  write_timeout: "5s"
  read_timeout: "5s"
  dial_timeout: "5s"

kafka:
  broker_list: "kafka-1:9092,kafka-2:9092,kafka-3:9092"
  enable: true
  consumer_group: "chat-service-group"
  client_id: "chat-service"
  env: ""                          # leave empty — producer uses bare topic names

centrifugo:
  api_url: "http://centrifugo:8000/api"
  api_key: "${CENTRIFUGO_API_KEY}"

keycloak:
  jwks_url: "https://auth.modami.com/realms/modami/protocol/openid-connect/certs"

observability:
  service_name: "chat-service"
  service_version: "1.0.0"
  environment: "production"
  log_level: "info"
  otlp_endpoint: "otel-collector:4317"
  otlp_insecure: false
```

---

## 9. Docker / Docker Compose

### Updated `docker-compose.yaml` (local dev, all services)

```yaml
services:
  scylladb:
    image: scylladb/scylla:6.2
    ports:
      - "9042:9042"
    command: >
      --smp 1
      --memory 512M
      --overprovisioned 1
      --developer-mode 1
    volumes:
      - scylladb_data:/var/lib/scylla
    healthcheck:
      test: ["CMD-SHELL", "cqlsh -e 'describe cluster' || exit 1"]
      interval: 15s
      timeout: 10s
      retries: 10
      start_period: 30s

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5

  kafka:
    image: bitnami/kafka:3.7
    ports:
      - "9092:9092"
    environment:
      KAFKA_CFG_NODE_ID: 0
      KAFKA_CFG_PROCESS_ROLES: controller,broker
      KAFKA_CFG_CONTROLLER_QUORUM_VOTERS: 0@kafka:9093
      KAFKA_CFG_LISTENERS: PLAINTEXT://:9092,CONTROLLER://:9093
      KAFKA_CFG_ADVERTISED_LISTENERS: PLAINTEXT://localhost:9092
      KAFKA_CFG_LISTENER_SECURITY_PROTOCOL_MAP: CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT
      KAFKA_CFG_CONTROLLER_LISTENER_NAMES: CONTROLLER
      KAFKA_CFG_AUTO_CREATE_TOPICS_ENABLE: "true"
    volumes:
      - kafka_data:/bitnami/kafka

  centrifugo:
    image: centrifugo/centrifugo:v5
    ports:
      - "8000:8000"
      - "10000:10000"   # admin UI
    volumes:
      - ./deploy/centrifugo/config.local.json:/centrifugo/config.json:ro
    command: centrifugo -c config.json
    environment:
      CENTRIFUGO_TOKEN_HMAC_SECRET_KEY: "${CENTRIFUGO_HMAC_SECRET}"
      CENTRIFUGO_API_KEY: "${CENTRIFUGO_API_KEY}"
      CENTRIFUGO_ADMIN_PASSWORD: "${CENTRIFUGO_ADMIN_PASSWORD}"
      CENTRIFUGO_ADMIN_SECRET: "${CENTRIFUGO_ADMIN_SECRET}"
    depends_on:
      redis:
        condition: service_healthy

  chat-service:
    build:
      context: .
      dockerfile: Dockerfile
    ports:
      - "8090:8090"
    environment:
      CHAT_CONFIG_PATH: /app/config/config-docker.yaml
    volumes:
      - ./config:/app/config:ro
    depends_on:
      scylladb:
        condition: service_healthy
      redis:
        condition: service_healthy
      kafka:
        condition: service_healthy
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:8090/health"]
      interval: 30s
      timeout: 3s
      retries: 3
      start_period: 10s

networks:
  default:
    driver: bridge

volumes:
  scylladb_data:
  redis_data:
  kafka_data:
```

### Build Image

```bash
# With GitLab private module access
docker build \
  --secret id=gitlab_username,src=<(echo "$GITLAB_USERNAME") \
  --secret id=gitlab_token,src=<(echo "$GITLAB_TOKEN") \
  -t chat-service:latest .

# Without private modules (public deps only)
docker build -t chat-service:latest .
```

---

## 10. Kubernetes Deployment

### Namespace

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: modami-chat
```

### ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: chat-service-config
  namespace: modami-chat
data:
  APP_ENVIRONMENT: "production"
  APP_PORT: "8090"
  SCYLLADB_HOSTS: "scylladb-0.scylladb,scylladb-1.scylladb,scylladb-2.scylladb"
  SCYLLADB_DATACENTER: "dc1"
  SCYLLADB_REPLICATION_FACTOR: "3"
  REDIS_HOST: "redis-master"
  REDIS_PORT: "6379"
  KAFKA_BROKER_LIST: "kafka-0.kafka:9092,kafka-1.kafka:9092,kafka-2.kafka:9092"
  KAFKA_CONSUMER_GROUP: "chat-service-group"
  CENTRIFUGO_API_URL: "http://centrifugo:8000/api"
  KEYCLOAK_JWKS_URL: "https://auth.modami.com/realms/modami/protocol/openid-connect/certs"
  OBSERVABILITY_LOG_LEVEL: "info"
  OBSERVABILITY_OTLP_ENDPOINT: "otel-collector:4317"
```

### Secret

```yaml
# secret.yaml — DO NOT commit to git
apiVersion: v1
kind: Secret
metadata:
  name: chat-service-secret
  namespace: modami-chat
type: Opaque
stringData:
  REDIS_PASSWORD: "CHANGE_ME"
  CENTRIFUGO_API_KEY: "CHANGE_ME"
```

### Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: chat-service
  namespace: modami-chat
spec:
  replicas: 2
  selector:
    matchLabels:
      app: chat-service
  template:
    metadata:
      labels:
        app: chat-service
    spec:
      terminationGracePeriodSeconds: 30
      containers:
        - name: chat-service
          image: registry.modami.com/chat-service:latest
          ports:
            - containerPort: 8090
              name: http
          envFrom:
            - configMapRef:
                name: chat-service-config
            - secretRef:
                name: chat-service-secret
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
            limits:
              cpu: 500m
              memory: 512Mi
          livenessProbe:
            httpGet:
              path: /health
              port: http
            initialDelaySeconds: 10
            periodSeconds: 15
          readinessProbe:
            httpGet:
              path: /health
              port: http
            initialDelaySeconds: 5
            periodSeconds: 10
---
apiVersion: v1
kind: Service
metadata:
  name: chat-service
  namespace: modami-chat
spec:
  selector:
    app: chat-service
  ports:
    - port: 8090
      targetPort: 8090
  type: ClusterIP
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: chat-service-hpa
  namespace: modami-chat
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: chat-service
  minReplicas: 2
  maxReplicas: 10
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: 80
```

### Centrifugo Deployment (K8s)

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: centrifugo
  namespace: modami-chat
spec:
  replicas: 2
  selector:
    matchLabels:
      app: centrifugo
  template:
    metadata:
      labels:
        app: centrifugo
    spec:
      containers:
        - name: centrifugo
          image: centrifugo/centrifugo:v5
          args: ["centrifugo", "-c", "/centrifugo/config.json"]
          ports:
            - containerPort: 8000
              name: http
            - containerPort: 8001
              name: websocket
          env:
            - name: CENTRIFUGO_TOKEN_HMAC_SECRET_KEY
              valueFrom:
                secretKeyRef:
                  name: centrifugo-secret
                  key: HMAC_SECRET
            - name: CENTRIFUGO_API_KEY
              valueFrom:
                secretKeyRef:
                  name: centrifugo-secret
                  key: API_KEY
          volumeMounts:
            - name: config
              mountPath: /centrifugo
      volumes:
        - name: config
          configMap:
            name: centrifugo-config
```

---

## 11. Environment Variables & Secrets

### chat-service

| Variable | Source | Description |
|----------|--------|-------------|
| `CHAT_CONFIG_PATH` | env | Path to config YAML (default: `config/config.yaml`) |
| `REDIS_PASSWORD` | secret | Redis auth password |
| `CENTRIFUGO_API_KEY` | secret | Centrifugo HTTP API key |

Config values can also be overridden via env vars using the `CHAT_` prefix (viper convention). Example:
```
CHAT_KAFKA_BROKER_LIST=kafka:9092
CHAT_SCYLLADB_HOSTS=scylladb:9042
```

### Centrifugo

| Variable | Description |
|----------|-------------|
| `CENTRIFUGO_TOKEN_HMAC_SECRET_KEY` | HMAC secret for JWT verification (⚠️ NOT `CENTRIFUGO_HMAC_SECRET`) |
| `CENTRIFUGO_API_KEY` | API key for server-side publish calls |
| `CENTRIFUGO_ADMIN_PASSWORD` | Admin UI password |
| `CENTRIFUGO_ADMIN_SECRET` | Admin UI HMAC secret |

---

## 12. Local Development

### Prerequisites

- Go 1.25+
- Docker + Docker Compose
- `air` for hot reload: `go install github.com/air-verse/air@latest`
- `swag` for Swagger: `go install github.com/swaggo/swag/cmd/swag@latest`

### Start Infrastructure

```bash
# Start ScyllaDB, Redis, Kafka
docker compose up -d scylladb redis kafka

# Start Centrifugo (with correct env vars)
docker run -d --name centrifugo \
  -p 8000:8000 -p 10000:10000 \
  -v $(pwd)/deploy/centrifugo/config.local.json:/centrifugo/config.json:ro \
  -e CENTRIFUGO_TOKEN_HMAC_SECRET_KEY=local-dev-token-secret \
  -e CENTRIFUGO_API_KEY=local-api-key \
  -e CENTRIFUGO_ADMIN_PASSWORD=admin \
  -e CENTRIFUGO_ADMIN_SECRET=admin-secret \
  centrifugo/centrifugo:v5 centrifugo -c config.json
```

### Create Kafka Topics (first time)

```bash
for topic in chat.messages.sent chat.messages.updated chat.messages.deleted chat.reactions chat.read_receipts chat.messages.inbound; do
  docker exec <kafka-container> kafka-topics \
    --bootstrap-server localhost:9092 \
    --create --if-not-exists \
    --topic "$topic" --partitions 1 --replication-factor 1
done
```

> If using `bitnami/kafka` with `KAFKA_CFG_AUTO_CREATE_TOPICS_ENABLE: "true"`, topics are created automatically when first produced to.

### Run Service

```bash
# With hot reload
make dev

# Or build + run
make run

# Or manually
CHAT_CONFIG_PATH=config/config.yaml go run ./cmd/server
```

### Verify Centrifugo Proxy Reachability

When Centrifugo runs in Docker and chat-service runs on the host, find the host IP:

```bash
# From inside the Centrifugo container
docker exec centrifugo wget -q -O- http://host.docker.internal:8090/health

# If host.docker.internal fails, find routable IP
docker run --rm alpine ip route show
# Use the 'default via X.X.X.X' IP in config.local.json
```

### Swagger UI

```
http://localhost:8090/swagger/index.html
```

### Regenerate Swagger Docs

```bash
make swagger
```

---

## 13. Critical Gotchas

### ScyllaDB 6.x — Tablets and LWT

**Problem:** ScyllaDB 6.x introduces "tablets" mode by default. LWT (`IF NOT EXISTS`) is **not supported** with tablets. The `chat.direct_conversations` table uses `INSERT ... IF NOT EXISTS` for dedup.

**Symptom:** `Cannot use LightWeight Transactions for table chat.direct_conversations: LWT is not yet supported with tablets`

**Fix:** Keyspace must be created with `AND tablets = {'enabled': false}`. This is handled automatically by `EnsureSchema`, but if you create the keyspace manually, you must include it.

---

### Kafka — Topic Prefix Inconsistency

**Problem:** `internal/adapter/messaging/producer.go` uses `EmitToFullTopic` (no env prefix), but `KafkaService.StartConsumer` uses `GetTopicWithEnv` (adds `{env}.` prefix from config). With `kafka.env: "local"`, the consumer subscribes to `local.chat.messages.inbound` but the producer sends to `chat.messages.sent`.

**Fix options:**
1. Set `kafka.env: ""` in config and create bare topic names (recommended for now)
2. Update `producer.go` to use `Emit` instead of `EmitToFullTopic` for consistent env-prefixed topic names

---

### Centrifugo v5 — Proxy Config Format

**Problem:** Centrifugo v5 changed the proxy configuration format from v4. The `proxies` array + `subscribe_proxy_name` pattern no longer works.

**Correct v5 format:**
```json
{
  "proxy_subscribe_endpoint": "http://...",   // top-level default
  "namespaces": [{
    "name": "chat",
    "proxy_subscribe": true,                  // enable for this namespace
    "proxy_subscribe_endpoint": "http://..."  // optional namespace override
  }]
}
```

---

### Centrifugo — JWT Secret Env Var

`CENTRIFUGO_HMAC_SECRET` does **not** map to `token_hmac_secret_key`. Use `CENTRIFUGO_TOKEN_HMAC_SECRET_KEY`.

---

### franz-go — `ProduceSync` Blocks on Missing Topic

`ProduceSync` blocks and retries for up to 30s when the topic doesn't exist. This causes HTTP handler timeouts. Ensure all topics exist before the service starts, or use `EnsureTopics()` at startup.

---

### ScyllaDB — First Write Latency

On a single-node dev instance with `--developer-mode 1`, the first write after startup can take 10–20s while ScyllaDB initializes compaction. This is normal in dev. In production with a properly sized cluster, writes are fast.
