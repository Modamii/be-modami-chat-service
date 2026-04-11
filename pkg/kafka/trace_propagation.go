package kafka

import (
	"context"

	"github.com/twmb/franz-go/pkg/kgo"
	pkgotel "gitlab.com/lifegoeson-libs/pkg-logging/otel"
)

type KafkaHeaderCarrier struct {
	headers []kgo.RecordHeader
}

func NewKafkaHeaderCarrier(headers []kgo.RecordHeader) *KafkaHeaderCarrier {
	return &KafkaHeaderCarrier{
		headers: headers,
	}
}

func (c *KafkaHeaderCarrier) Get(key string) string {
	for _, header := range c.headers {
		if header.Key == key {
			return string(header.Value)
		}
	}
	return ""
}

func (c *KafkaHeaderCarrier) Set(key, value string) {
	for i := range c.headers {
		if c.headers[i].Key == key {
			c.headers[i].Value = []byte(value)
			return
		}
	}
	c.headers = append(c.headers, kgo.RecordHeader{
		Key:   key,
		Value: []byte(value),
	})
}

func (c *KafkaHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c.headers))
	for _, header := range c.headers {
		keys = append(keys, header.Key)
	}
	return keys
}

func (c *KafkaHeaderCarrier) Headers() []kgo.RecordHeader {
	return c.headers
}

func InjectTraceContext(ctx context.Context, headers []kgo.RecordHeader) []kgo.RecordHeader {
	carrier := NewKafkaHeaderCarrier(headers)
	pkgotel.Inject(ctx, carrier)
	return carrier.Headers()
}

func ExtractTraceContext(ctx context.Context, headers []kgo.RecordHeader) context.Context {
	carrier := NewKafkaHeaderCarrier(headers)
	return pkgotel.Extract(ctx, carrier)
}
