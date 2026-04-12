# WebSocket & Centrifugo

## Architecture: 1 Centrifugo, 2 Namespaces

One Centrifugo instance is shared between the notification service and the chat service. Clients open **a single WebSocket connection** and subscribe to channels from both namespaces.

```
                        ┌─────────────────────────────────┐
                        │         Centrifugo               │
                        │                                 │
  Client ──── 1 WS ────▶│  noti:user:{id}    (noti ns)   │
                        │  chat:room:{id}    (chat ns)    │
                        └────────────┬────────────────────┘
                                     │ proxy callbacks (per namespace)
                     ┌───────────────┴──────────────────┐
                     ▼                                  ▼
          noti-ws-gateway :7072              chat-ws-gateway
          (this service)                    (chat service)
```

Why 1 Centrifugo instead of 2?
- Client opens only 1 WebSocket — no double handshake, no double JWT
- Single auth flow — 1 token issued by auth service
- No duplicate connection management
- Split only when: different teams own each service, wildly different scale needs, or strict security isolation is required

---

## Channel Naming

Centrifugo uses `namespace:channel` pattern:

| Channel | Namespace | Who subscribes | Who publishes |
|---------|-----------|----------------|---------------|
| `noti:user:{userID}` | `noti` | User (personal) | noti-service |
| `noti:topic:{topic}` | `noti` | Anyone | noti-service (broadcast) |
| `chat:room:{roomID}` | `chat` | Room members | chat-service |

---

## Namespace Config

```json
{
  "namespaces": [
    {
      "name": "noti",
      "presence": false,
      "history_size": 20,
      "history_ttl": "7d",
      "recover": true,
      "proxy_subscribe_endpoint": "http://noti-ws-gateway:7072/centrifugo/subscribe",
      "proxy_publish_endpoint":   "http://noti-ws-gateway:7072/centrifugo/publish"
    },
    {
      "name": "chat",
      "presence": true,
      "join_leave": true,
      "history_size": 100,
      "history_ttl": "24h",
      "recover": true,
      "proxy_subscribe_endpoint": "http://chat-ws-gateway:PORT/centrifugo/subscribe",
      "proxy_publish_endpoint":   "http://chat-ws-gateway:PORT/centrifugo/publish"
    }
  ]
}
```

- `noti` — no presence (no need to know who's online), long history TTL for missed notifications
- `chat` — presence enabled (know who's in the room), join/leave events, shorter history

---

## Full Connection Flow

```
1. Client → GET /auth/centrifugo-token  (noti-service API)
   ← { "token": "eyJ..." }            (JWT signed with HMAC secret)

2. Client → Centrifugo WS connect
   sends:  { "token": "eyJ..." }
   Centrifugo → POST /centrifugo/connect  (noti-ws-gateway)
   ← { "result": { "user": "user-123", "channels": ["noti:user:user-123"] } }
   Client is now connected + auto-subscribed to noti channel

3. Client → subscribe "chat:room:room-456"
   Centrifugo → POST /centrifugo/subscribe  (chat-ws-gateway)
   ← { "result": {} }   (chat service validates membership)

4. Server publishes notification:
   noti-service → POST centrifugo/api  { "method": "publish", "params": { "channel": "noti:user:user-123", ... } }
   Centrifugo → client receives message on WS

5. User sends chat message:
   chat-service → POST centrifugo/api  { "method": "publish", "params": { "channel": "chat:room:room-456", ... } }
   Centrifugo → all subscribers in room receive message on same WS connection
```

---

## JWT Token

Issued by `POST /v1/noti-services/auth/centrifugo-token`:

```json
{
  "sub": "user-123",
  "exp": 1234567890,
  "iat": 1234564290
}
```

- Signed with HMAC-SHA256 using `centrifugo.hmac_secret`
- Same secret shared between noti-service and Centrifugo
- Chat service does NOT need to issue its own token — same JWT works for both namespaces

---

## Proxy Routing

Centrifugo routes proxy callbacks per namespace:

| Event | Endpoint | Handler |
|-------|----------|---------|
| Connect (all namespaces) | `noti-ws-gateway:7072/centrifugo/connect` | noti-service |
| Subscribe `noti:*` | `noti-ws-gateway:7072/centrifugo/subscribe` | noti-service |
| Publish `noti:*` | `noti-ws-gateway:7072/centrifugo/publish` | noti-service |
| Subscribe `chat:*` | `chat-ws-gateway:PORT/centrifugo/subscribe` | chat-service |
| Publish `chat:*` | `chat-ws-gateway:PORT/centrifugo/publish` | chat-service |

Connect proxy is handled by noti-service because it only validates JWT — no service-specific logic.

---

## Subscribe Access Rules (noti namespace)

| Channel | Rule |
|---------|------|
| `noti:user:{X}` | Only user with `sub == X` can subscribe |
| `noti:topic:*` | Any authenticated user |

Chat subscribe rules are enforced by the chat service.

---

## Rate Limiting

Client-initiated publishes to `noti:*` are rate-limited per user:
- 10 publishes/second, burst of 20

Server-initiated publishes (via Centrifugo HTTP API) are not rate-limited here.

---

## JavaScript Client Example

```js
import { Centrifuge } from 'centrifuge';

const token = await fetch('/v1/noti-services/auth/centrifugo-token', {
  method: 'POST',
  body: JSON.stringify({ user_id: 'user-123' }),
}).then(r => r.json()).then(r => r.data.token);

const centrifuge = new Centrifuge('wss://centrifugo.app.modami.com/connection/websocket', {
  data: { token }   // sent in connect proxy request
});

// Subscribe to personal notifications (auto-subscribed on connect too)
const notiSub = centrifuge.newSubscription('noti:user:user-123');
notiSub.on('publication', ({ data }) => {
  console.log('notification:', data);
});
notiSub.subscribe();

// Subscribe to a chat room
const chatSub = centrifuge.newSubscription('chat:room:room-456');
chatSub.on('publication', ({ data }) => {
  console.log('chat message:', data);
});
chatSub.subscribe();

centrifuge.connect();
```

---

## Local Development

```bash
# Start Centrifugo with local config (both namespaces)
docker run -d --name centrifugo \
  -p 8000:8000 -p 10000:10000 \
  -v $(pwd)/deploy/centrifugo/config.local.json:/centrifugo/config.json \
  -e CENTRIFUGO_HMAC_SECRET=local-secret \
  -e CENTRIFUGO_API_KEY=local-api-key \
  -e CENTRIFUGO_ADMIN_PASSWORD=admin \
  -e CENTRIFUGO_ADMIN_SECRET=admin-secret \
  centrifugo/centrifugo:v5 centrifugo --config=config.json

# Run noti ws-gateway
make run-ws-gateway   # :7072

# Admin UI
open http://localhost:10000
```
