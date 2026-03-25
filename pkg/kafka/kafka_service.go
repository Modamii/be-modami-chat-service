package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	config "github.com/techinsight/be-techinsights-notification-service/configs"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	logging "gitlab.com/lifegoeson-libs/pkg-logging"
	"gitlab.com/lifegoeson-libs/pkg-logging/logger"
)
type KafkaService struct {
	config    *KafkaConfig
	appConfig *config.Config
	client    *kgo.Client
	topics    *KafkaTopics
	mu        sync.RWMutex
	running   bool
}
func NewKafkaService(cfg *KafkaConfig, appCfg *config.Config) (*KafkaService, error) {
	if cfg == nil {
		cfg = GetDefaultKafkaConfig(appCfg)
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
		config:    cfg,
		appConfig: appCfg,
		client:    client,
		topics:    GetKafkaTopics(),
		running:   false,
	}, nil
}
type ProducerMessage struct {
	Key     string                 `json:"key"`
	Value   interface{}            `json:"value"`
	Headers map[string]interface{} `json:"headers,omitempty"`
}
// Emit sends a message to Kafka synchronously with trace context propagation
func (k *KafkaService) Emit(ctx context.Context, topic string, message *ProducerMessage) error {
	topicName := GetTopicWithEnv(k.appConfig, topic)
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
		logger.Error(ctx, "Failed to send message", err,
			logging.String("topic", topicName),
			logging.String("key", message.Key),
		)
		return fmt.Errorf("failed to send message to topic %s: %w", topicName, err)
	}

	logger.Info(ctx, "Message sent successfully",
		logging.String("topic", topicName),
		logging.String("key", message.Key),
	)
	return nil
}

func (k *KafkaService) EnsureTopics(ctx context.Context) error {
	adm := kadm.NewClient(k.client)

	baseTopics := GetAllTopics()
	targetTopics := make([]string, 0, len(baseTopics))
	for _, t := range baseTopics {
		targetTopics = append(targetTopics, GetTopicWithEnv(k.appConfig, t))
	}

	logger.Info(ctx, "Ensuring Kafka topics exist...",
		logging.Int("count", len(targetTopics)),
	)

	metadata, err := adm.Metadata(ctx)
	if err != nil {
		return fmt.Errorf("failed to get kafka metadata: %w", err)
	}

	brokerCount := len(metadata.Brokers)
	replicationFactor := int16(3)
	if brokerCount < 3 {
		replicationFactor = 1
		logger.Warn(ctx, "Broker count is less than 3, falling back to lower replication factor",
			logging.Int("brokers", brokerCount),
			logging.Int("fallback_replication", int(replicationFactor)),
		)
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

	envPrefix := GetTopicWithEnv(k.appConfig, "")
	redundantTopics := make([]string, 0)
	for _, t := range existingTopics.Names() {
		if strings.HasPrefix(t, envPrefix) {
			isTarget := false
			for _, target := range targetTopics {
				if t == target {
					isTarget = true
					break
				}
			}
			if !isTarget {
				redundantTopics = append(redundantTopics, t)
			}
		}
	}

	if len(redundantTopics) > 0 {
		logger.Info(ctx, "Deleting redundant Kafka topics...",
			logging.Int("count", len(redundantTopics)),
			logging.String("topics", strings.Join(redundantTopics, ",")),
		)
		delResp, err := adm.DeleteTopics(ctx, redundantTopics...)
		if err != nil {
			logger.Error(ctx, "Failed to delete redundant topics", err)
		} else {
			for _, res := range delResp {
				if res.Err != nil {
					logger.Error(ctx, "Failed to delete redundant topic", res.Err,
						logging.String("topic", res.Topic),
					)
				} else {
					logger.Info(ctx, "Successfully deleted redundant topic",
						logging.String("topic", res.Topic),
					)
				}
			}
		}
	}

	if len(missingTopics) == 0 {
		logger.Info(ctx, "All required Kafka topics already exist")
		return nil
	}

	logger.Info(ctx, "Creating missing Kafka topics...",
		logging.Int("missing_count", len(missingTopics)),
		logging.Int("partitions", 1),
		logging.Int("replication", int(replicationFactor)),
	)

	resp, err := adm.CreateTopics(ctx, 1, replicationFactor, nil, missingTopics...)
	if err != nil {
		return fmt.Errorf("failed to create missing topics: %w", err)
	}

	hasError := false
	for _, res := range resp {
		if res.Err != nil {
			logger.Error(ctx, "Failed to create topic", res.Err,
				logging.String("topic", res.Topic),
			)
			hasError = true
		} else {
			logger.Info(ctx, "Successfully created topic",
				logging.String("topic", res.Topic),
			)
		}
	}

	if hasError {
		return fmt.Errorf("some topics failed to be created")
	}
	return nil
}

func (k *KafkaService) EmitAsync(ctx context.Context, topic string, message *ProducerMessage) {
	topicName := GetTopicWithEnv(k.appConfig, topic)
	valueBytes, err := json.Marshal(message.Value)
	if err != nil {
		logger.Error(ctx, "Failed to marshal message value for async emit", err)
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

	// Detach from the request context so the produce is not canceled
	// when the HTTP handler returns. Use a timeout to avoid leaking.
	produceCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)

	k.client.Produce(produceCtx, record, func(r *kgo.Record, err error) {
		defer cancel()
		if err != nil {
			logger.Error(produceCtx, "Failed to send async message", err,
				logging.String("topic", topicName),
				logging.String("key", message.Key),
			)
			return
		}
		logger.Debug(produceCtx, "Async message sent successfully",
			logging.String("topic", topicName),
			logging.String("key", message.Key),
		)
	})
}

func (k *KafkaService) SendMessages(ctx context.Context, topic string, messages []*ProducerMessage) error {
	topicName := GetTopicWithEnv(k.appConfig, topic)
	records := make([]*kgo.Record, 0, len(messages))

	for _, message := range messages {
		valueBytes, err := json.Marshal(message.Value)
		if err != nil {
			logger.Error(ctx, "Failed to marshal message value", err)
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
		logger.Error(ctx, "Failed to send messages", err,
			logging.String("topic", topicName),
			logging.Int("count", len(messages)),
		)
		return fmt.Errorf("failed to send messages to topic %s: %w", topicName, err)
	}

	logger.Info(ctx, "Messages sent successfully",
		logging.String("topic", topicName),
		logging.Int("count", len(messages)),
	)
	return nil
}

func (k *KafkaService) buildHeaders(ctx context.Context, customHeaders map[string]interface{}) []kgo.RecordHeader {
	headers := make([]kgo.RecordHeader, 0, len(customHeaders)+1)

	if requestID := getRequestIDFromContext(ctx); requestID != "" {
		headers = append(headers, kgo.RecordHeader{
			Key:   "request-id",
			Value: []byte(requestID),
		})
	}

	for key, value := range customHeaders {
		headerBytes, err := json.Marshal(value)
		if err != nil {
			logger.Warn(ctx, "Failed to marshal header value",
				logging.String("key", key),
				logging.String("error", err.Error()),
			)
			continue
		}
		headers = append(headers, kgo.RecordHeader{
			Key:   key,
			Value: headerBytes,
		})
	}

	return headers
}
func (k *KafkaService) RegisterHandler(handler ConsumerHandler) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.running {
		logger.Error(context.Background(), "Cannot register handler while service is running", fmt.Errorf("service running"))
		return
	}
	logger.Info(context.Background(), "Registering consumer handler",
		logging.String("topics", strings.Join(handler.GetTopics(), ",")),
	)
}

func (k *KafkaService) StartConsumer(ctx context.Context, handlers []ConsumerHandler) error {
	k.mu.Lock()
	if k.running {
		k.mu.Unlock()
		return fmt.Errorf("consumer is already running")
	}
	k.running = true
	k.mu.Unlock()

	handlerMap := make(map[string][]ConsumerHandler)
	var topics []string
	topicSet := make(map[string]struct{})

	for _, handler := range handlers {
		for _, topic := range handler.GetTopics() {
			topicName := GetTopicWithEnv(k.appConfig, topic)
			if _, exists := topicSet[topicName]; !exists {
				topicSet[topicName] = struct{}{}
				topics = append(topics, topicName)
			}
			handlerMap[topicName] = append(handlerMap[topicName], handler)
		}
	}

	k.client.AddConsumeTopics(topics...)
	logger.Info(ctx, "Starting consumer group",
		logging.String("topics", strings.Join(topics, ",")),
	)

	for {
		fetches := k.client.PollFetches(ctx)
		if err := fetches.Err(); err != nil {
			if err == context.Canceled {
				logger.Info(ctx, "Consumer context cancelled")
				k.mu.Lock()
				k.running = false
				k.mu.Unlock()
				return nil
			}
			logger.Error(ctx, "Consumer poll error", err)
			time.Sleep(time.Second)
			continue
		}

		iter := fetches.RecordIter()
		for !iter.Done() {
			record := iter.Next()

			msgCtx := k.extractContextFromHeaders(record.Headers)

			handlers, exists := handlerMap[record.Topic]
			if !exists {
				logger.Warn(msgCtx, "No handlers found for topic",
					logging.String("topic", record.Topic),
				)
				continue
			}

			for _, handler := range handlers {
				if err := handler.HandleMessage(msgCtx, record); err != nil {
					logger.Error(msgCtx, "Failed to handle message", err,
						logging.String("topic", record.Topic),
					)
				}
			}
		}
	}
}
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

func (k *KafkaService) GetTopics() *KafkaTopics {
	return k.topics
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
		return fmt.Errorf("failed to send test message: %w", err)
	}
	return nil
}
