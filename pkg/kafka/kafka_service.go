package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	logging "gitlab.com/lifegoeson-libs/pkg-logging"
	"gitlab.com/lifegoeson-libs/pkg-logging/logger"
)

type KafkaService struct {
	config  *KafkaConfig
	env     string
	client  *kgo.Client
	mu      sync.RWMutex
	running bool
}

func NewKafkaService(cfg *KafkaConfig, env string) (*KafkaService, error) {
	if cfg == nil {
		return nil, fmt.Errorf("kafka config is required")
	}

	opts, err := cfg.ToFranzGoOpts()
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka options: %w", err)
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka client: %w", err)
	}

	return &KafkaService{
		config:  cfg,
		env:     env,
		client:  client,
		running: false,
	}, nil
}

type ProducerMessage struct {
	Key     string                 `json:"key"`
	Value   interface{}            `json:"value"`
	Headers map[string]interface{} `json:"headers,omitempty"`
}

// Emit sends a message to Kafka synchronously with trace context propagation.
func (k *KafkaService) Emit(ctx context.Context, topic string, message *ProducerMessage) error {
	topicName := GetTopicWithEnv(k.env, topic)

	valueBytes, err := json.Marshal(message.Value)
	if err != nil {
		return fmt.Errorf("failed to marshal message value: %w", err)
	}

	headers := k.buildHeaders(ctx, message.Headers)
	headers = InjectTraceContext(ctx, headers)

	record := &kgo.Record{
		Topic:   topicName,
		Key:     []byte(message.Key),
		Value:   valueBytes,
		Headers: headers,
	}

	if err := k.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		logger.Error(ctx, "failed to send message", err, logging.String("topic", topicName), logging.String("key", message.Key))
		return fmt.Errorf("failed to send message to topic %s: %w", topicName, err)
	}

	logger.Info(ctx, "message sent successfully", logging.String("topic", topicName), logging.String("key", message.Key))
	return nil
}

// EmitToFullTopic sends a message to an already fully-qualified topic name (no env prefix added).
func (k *KafkaService) EmitToFullTopic(ctx context.Context, topic string, message *ProducerMessage) error {
	valueBytes, err := json.Marshal(message.Value)
	if err != nil {
		return fmt.Errorf("failed to marshal message value: %w", err)
	}

	headers := k.buildHeaders(ctx, message.Headers)
	headers = InjectTraceContext(ctx, headers)

	record := &kgo.Record{
		Topic:   topic,
		Key:     []byte(message.Key),
		Value:   valueBytes,
		Headers: headers,
	}

	if err := k.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		return fmt.Errorf("failed to send message to topic %s: %w", topic, err)
	}
	return nil
}

// EmitAsync sends a message asynchronously, detached from the request context.
func (k *KafkaService) EmitAsync(ctx context.Context, topic string, message *ProducerMessage) {
	topicName := GetTopicWithEnv(k.env, topic)

	valueBytes, err := json.Marshal(message.Value)
	if err != nil {
		logger.Error(ctx, "failed to marshal message value for async emit", err)
		return
	}

	headers := k.buildHeaders(ctx, message.Headers)
	headers = InjectTraceContext(ctx, headers)

	record := &kgo.Record{
		Topic:   topicName,
		Key:     []byte(message.Key),
		Value:   valueBytes,
		Headers: headers,
	}

	produceCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)

	k.client.Produce(produceCtx, record, func(r *kgo.Record, err error) {
		defer cancel()
		if err != nil {
			logger.Error(produceCtx, "failed to send async message", err, logging.String("topic", topicName), logging.String("key", message.Key))
			return
		}
		logger.Debug(produceCtx, "async message sent successfully", logging.String("topic", topicName), logging.String("key", message.Key))
	})
}

// SendMessages sends multiple messages to a topic synchronously.
func (k *KafkaService) SendMessages(ctx context.Context, topic string, messages []*ProducerMessage) error {
	topicName := GetTopicWithEnv(k.env, topic)
	records := make([]*kgo.Record, 0, len(messages))

	for _, message := range messages {
		valueBytes, err := json.Marshal(message.Value)
		if err != nil {
			logger.Error(ctx, "failed to marshal message value", err)
			continue
		}

		headers := k.buildHeaders(ctx, message.Headers)
		headers = InjectTraceContext(ctx, headers)

		records = append(records, &kgo.Record{
			Topic:   topicName,
			Key:     []byte(message.Key),
			Value:   valueBytes,
			Headers: headers,
		})
	}

	if err := k.client.ProduceSync(ctx, records...).FirstErr(); err != nil {
		logger.Error(ctx, "failed to send messages", err, logging.String("topic", topicName), logging.Int("count", len(messages)))
		return fmt.Errorf("failed to send messages to topic %s: %w", topicName, err)
	}

	logger.Info(ctx, "messages sent successfully", logging.String("topic", topicName), logging.Int("count", len(messages)))
	return nil
}

// EnsureTopics creates missing topics and (when env is set) removes unknown ones.
func (k *KafkaService) EnsureTopics(ctx context.Context) error {
	adm := kadm.NewClient(k.client)

	targetTopics := GetAllTopics(k.env)

	logger.Info(ctx, "ensuring kafka topics exist", logging.Int("count", len(targetTopics)))

	metadata, err := adm.Metadata(ctx)
	if err != nil {
		return fmt.Errorf("failed to get kafka metadata: %w", err)
	}

	brokerCount := len(metadata.Brokers)
	replicationFactor := int16(3)
	if brokerCount < 3 {
		replicationFactor = 1
		logger.Warn(ctx, "broker count < 3, using replication factor 1", logging.Int("brokers", brokerCount))
	}

	existingTopics, err := adm.ListTopics(ctx)
	if err != nil {
		return fmt.Errorf("failed to list kafka topics: %w", err)
	}

	missingTopics := make([]string, 0)
	for _, t := range targetTopics {
		if !existingTopics.Has(t) {
			missingTopics = append(missingTopics, t)
		}
	}

	if len(missingTopics) == 0 {
		logger.Info(ctx, "all required kafka topics already exist")
		return nil
	}

	logger.Info(ctx, "creating missing kafka topics", logging.Int("count", len(missingTopics)), logging.Any("replication", replicationFactor))
	resp, err := adm.CreateTopics(ctx, 1, replicationFactor, nil, missingTopics...)
	if err != nil {
		return fmt.Errorf("failed to create missing topics: %w", err)
	}

	hasError := false
	for _, res := range resp {
		if res.Err != nil {
			logger.Error(ctx, "failed to create topic", res.Err, logging.String("topic", res.Topic))
			hasError = true
		} else {
			logger.Info(ctx, "created topic", logging.String("topic", res.Topic))
		}
	}

	if hasError {
		return fmt.Errorf("some topics failed to be created")
	}
	return nil
}

// StartConsumer runs the consumer loop, dispatching each record to registered handlers.
// Blocks until ctx is cancelled.
func (k *KafkaService) StartConsumer(ctx context.Context, handlers []ConsumerHandler) error {
	k.mu.Lock()
	if k.running {
		k.mu.Unlock()
		return fmt.Errorf("consumer is already running")
	}
	k.running = true
	k.mu.Unlock()

	defer func() {
		k.mu.Lock()
		k.running = false
		k.mu.Unlock()
	}()

	handlerMap := make(map[string][]ConsumerHandler)
	var topics []string
	topicSet := make(map[string]struct{})

	for _, handler := range handlers {
		for _, topic := range handler.GetTopics() {
			topicName := GetTopicWithEnv(k.env, topic)
			if _, exists := topicSet[topicName]; !exists {
				topicSet[topicName] = struct{}{}
				topics = append(topics, topicName)
			}
			handlerMap[topicName] = append(handlerMap[topicName], handler)
		}
	}

	k.client.AddConsumeTopics(topics...)
	logger.Info(ctx, "starting consumer group", logging.String("topics", strings.Join(topics, ",")))

	for {
		fetches := k.client.PollFetches(ctx)
		if err := fetches.Err(); err != nil {
			if ctx.Err() != nil {
				logger.Info(ctx, "consumer context cancelled")
				return nil
			}
			if strings.Contains(err.Error(), "UNKNOWN_TOPIC_ID") {
				logger.Warn(ctx, "unknown topic ID detected, refreshing metadata and re-subscribing")
				k.client.ForceMetadataRefresh()
				k.client.AddConsumeTopics(topics...)
				time.Sleep(2 * time.Second)
				continue
			}
			logger.Error(ctx, "consumer poll error", err)
			time.Sleep(time.Second)
			continue
		}

		iter := fetches.RecordIter()
		for !iter.Done() {
			record := iter.Next()
			msgCtx := k.extractContextFromHeaders(record.Headers)
			hs, exists := handlerMap[record.Topic]
			if !exists {
				logger.Warn(msgCtx, "no handlers found for topic", logging.String("topic", record.Topic))
				continue
			}
			for _, h := range hs {
				if err := h.HandleMessage(msgCtx, record); err != nil {
					logger.Error(msgCtx, "failed to handle message", err, logging.String("topic", record.Topic))
				}
			}
		}
	}
}

// Close shuts down the Kafka client.
func (k *KafkaService) Close() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.client.Close()
	k.running = false
	return nil
}

func (k *KafkaService) IsRunning() bool {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.running
}

func (k *KafkaService) Ping(ctx context.Context) error {
	if k.client == nil {
		return fmt.Errorf("kafka client is not initialized")
	}
	record := &kgo.Record{
		Topic: "__healthcheck",
		Key:   []byte("health"),
		Value: []byte("ping"),
	}
	if err := k.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		return fmt.Errorf("kafka ping failed: %w", err)
	}
	return nil
}

func (k *KafkaService) buildHeaders(ctx context.Context, customHeaders map[string]interface{}) []kgo.RecordHeader {
	headers := make([]kgo.RecordHeader, 0, len(customHeaders)+1)

	if requestID := getRequestIDFromContext(ctx); requestID != "" {
		headers = append(headers, kgo.RecordHeader{Key: "request-id", Value: []byte(requestID)})
	}

	for key, value := range customHeaders {
		headerBytes, err := json.Marshal(value)
		if err != nil {
			logger.Warn(ctx, "failed to marshal header value", logging.String("key", key), logging.String("error", err.Error()))
			continue
		}
		headers = append(headers, kgo.RecordHeader{Key: key, Value: headerBytes})
	}

	return headers
}

func (k *KafkaService) extractContextFromHeaders(headers []kgo.RecordHeader) context.Context {
	ctx := context.Background()
	ctx = ExtractTraceContext(ctx, headers)
	for _, header := range headers {
		if header.Key == "request-id" {
			ctx = setRequestIDInContext(ctx, string(header.Value))
			break
		}
	}
	return ctx
}

type requestIDKeyType string

const requestIDCtxKey requestIDKeyType = "kafka_request_id"

func getRequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDCtxKey).(string); ok {
		return id
	}
	return ""
}

func setRequestIDInContext(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDCtxKey, id)
}
