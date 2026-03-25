package centrifugo

import "encoding/json"

// Centrifugo proxy protocol types.
// See: https://centrifugal.dev/docs/server/proxy

// ProxyConnectRequest is sent by Centrifugo when a client connects.
type ProxyConnectRequest struct {
	Client    string          `json:"client"`
	Transport string          `json:"transport"`
	Protocol  string          `json:"protocol"`
	Encoding  string          `json:"encoding"`
	Data      json.RawMessage `json:"data,omitempty"` // custom data from client on connect
	Name      string          `json:"name,omitempty"` // client SDK name
	Version   string          `json:"version,omitempty"`
}

// ProxyConnectResult is returned to Centrifugo on successful connect.
type ProxyConnectResult struct {
	User     string          `json:"user"`               // user ID
	ExpireAt int64           `json:"expire_at,omitempty"` // unix timestamp; 0 = no expiry
	Channels []string        `json:"channels,omitempty"`  // auto-subscribe channels
	Data     json.RawMessage `json:"data,omitempty"`      // custom data sent to client
}

// ProxySubscribeRequest is sent by Centrifugo when a client subscribes to a channel.
type ProxySubscribeRequest struct {
	Client    string `json:"client"`
	Transport string `json:"transport"`
	Protocol  string `json:"protocol"`
	Encoding  string `json:"encoding"`
	User      string `json:"user"`
	Channel   string `json:"channel"`
	Token     string `json:"token,omitempty"`
}

// ProxySubscribeResult is returned to Centrifugo on successful subscribe.
type ProxySubscribeResult struct {
	ExpireAt int64           `json:"expire_at,omitempty"`
	Info     json.RawMessage `json:"info,omitempty"` // channel info attached to the subscription
}

// ProxyPublishRequest is sent by Centrifugo when a client publishes to a channel.
type ProxyPublishRequest struct {
	Client    string          `json:"client"`
	Transport string          `json:"transport"`
	Protocol  string          `json:"protocol"`
	Encoding  string          `json:"encoding"`
	User      string          `json:"user"`
	Channel   string          `json:"channel"`
	Data      json.RawMessage `json:"data"`
}

// ProxyPublishResult is returned to Centrifugo on successful publish.
type ProxyPublishResult struct {
	Data json.RawMessage `json:"data,omitempty"` // modified data; nil = use original
}

// ProxyResponse is the unified response envelope for all proxy callbacks.
type ProxyResponse struct {
	Result interface{} `json:"result,omitempty"`
	Error  *ProxyError `json:"error,omitempty"`
}

// ProxyError tells Centrifugo to reject the operation.
type ProxyError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Disconnect optionally tells Centrifugo to disconnect the client.
type Disconnect struct {
	Code   int    `json:"code"`
	Reason string `json:"reason"`
}

// Common proxy error codes (Centrifugo convention).
const (
	ProxyErrorUnauthorized = 401
	ProxyErrorForbidden    = 403
	ProxyErrorTooMany      = 429
	ProxyErrorInternal     = 500
)
