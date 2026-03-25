package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	logging "gitlab.com/lifegoeson-libs/pkg-logging"
	"gitlab.com/lifegoeson-libs/pkg-logging/logger"
)
// RunConsumer creates a simple Kafka consumer that processes messages with a callback.
// This is a convenience function for services that need a straightforward consumer loop.
func RunConsumer(ctx context.Context, brokers []string, groupID, topic string, fn func(ctx context.Context, key, value []byte) error) error {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(groupID),
		kgo.ConsumeTopics(topic),
	)
	if err != nil {
		return fmt.Errorf("failed to create kafka client: %w", err)
	}
	defer client.Close()

	for {
		fetches := client.PollFetches(ctx)
		if err := ctx.Err(); err != nil {
			return err
		}
		iter := fetches.RecordIter()
		for !iter.Done() {
			record := iter.Next()
			if err := fn(ctx, record.Key, record.Value); err != nil {
				logger.Error(ctx, "consumer handler error", err,
					logging.String("topic", record.Topic),
				)
			}
		}
	}
}

type HandlerFunc func(ctx context.Context, payload interface{}) error
type ConsumerOptions struct {
	RetryCount    int
	RetryDelay    time.Duration
	DeadLetterTopic string
	EnableLogging bool
}
func DefaultConsumerOptions() *ConsumerOptions {
	return &ConsumerOptions{
		RetryCount:    3,
		RetryDelay:    time.Second,
		DeadLetterTopic: "",
		EnableLogging: true,
	}
}
type TopicHandler struct {
	Topic       string
	Handler     HandlerFunc
	PayloadType reflect.Type
	Options     *ConsumerOptions
}
func NewTopicHandler(topic string, handler HandlerFunc, payloadType reflect.Type, options *ConsumerOptions) *TopicHandler {
	if options == nil {
		options = DefaultConsumerOptions()
	}
	return &TopicHandler{
		Topic:       topic,
		Handler:     handler,
		PayloadType: payloadType,
		Options:     options,
	}
}
func EventPatternAndLog(topic string, payloadType reflect.Type, options *ConsumerOptions) func(HandlerFunc) *TopicHandler {
	return func(handler HandlerFunc) *TopicHandler {
		return NewTopicHandler(topic, handler, payloadType, options)
	}
}
type Consumer struct {
	name         string
	handlers     map[string]*TopicHandler
	kafkaService *KafkaService
}

func NewConsumer(name string, kafkaService *KafkaService) *Consumer {
	return &Consumer{
		name:         name,
		handlers:     make(map[string]*TopicHandler),
		kafkaService: kafkaService,
	}
}
func (c *Consumer) RegisterHandler(handler *TopicHandler) {
	c.handlers[handler.Topic] = handler
	logger.Info(context.Background(), "Registered topic handler",
		logging.String("consumer", c.name),
		logging.String("topic", handler.Topic),
	)
}
func (c *Consumer) HandleMessage(ctx context.Context, message *kgo.Record) error {
	topic := message.Topic
	handler, exists := c.handlers[topic]
	if !exists {
		logger.Warn(ctx, "No handler found for topic",
			logging.String("consumer", c.name),
			logging.String("topic", topic),
		)
		return fmt.Errorf("no handler found for topic: %s", topic)
	}
	if handler.Options.EnableLogging {
		logger.Info(ctx, "EventPattern start",
			logging.String("consumer", c.name),
			logging.String("topic", topic),
		)
	}
	start := time.Now()
	var lastErr error
	for attempt := 0; attempt <= handler.Options.RetryCount; attempt++ {
		if attempt > 0 {
			logger.Warn(ctx, "Retrying message processing",
				logging.String("consumer", c.name),
				logging.String("topic", topic),
				logging.Int("attempt", attempt),
			)
			time.Sleep(handler.Options.RetryDelay)
		}
		err := c.processMessage(ctx, handler, message)
		if err == nil {
			if handler.Options.EnableLogging {
				logger.Info(ctx, "EventPattern done",
					logging.String("consumer", c.name),
					logging.String("topic", topic),
					logging.String("duration", time.Since(start).String()),
				)
			}
			return nil
		}
		lastErr = err
		logger.Error(ctx, "EventPattern error", err,
			logging.String("consumer", c.name),
			logging.String("topic", topic),
			logging.Int("attempt", attempt+1),
		)
	}
	logger.Error(ctx, "EventPattern failed after all retries", lastErr,
		logging.String("consumer", c.name),
		logging.String("topic", topic),
		logging.Int("totalAttempts", handler.Options.RetryCount+1),
		logging.String("totalDuration", time.Since(start).String()),
	)
	if handler.Options.DeadLetterTopic != "" {
		if err := c.sendToDeadLetterTopic(ctx, handler.Options.DeadLetterTopic, message, lastErr); err != nil {
			logger.Error(ctx, "Failed to send message to dead letter topic", err,
				logging.String("consumer", c.name),
				logging.String("deadLetterTopic", handler.Options.DeadLetterTopic),
			)
		}
	}
	return lastErr
}
func (c *Consumer) processMessage(ctx context.Context, handler *TopicHandler, message *kgo.Record) error {
	payload, err := c.deserializePayload(handler, message.Value)
	if err != nil {
		return fmt.Errorf("failed to deserialize payload: %w", err)
	}
	defer func() {
		if r := recover(); r != nil {
			logger.Error(ctx, "Handler panicked", fmt.Errorf("panic: %v", r),
				logging.String("consumer", c.name),
				logging.String("topic", handler.Topic),
				logging.String("panic", fmt.Sprint(r)),
				logging.String("stack", getStackTrace()),
			)
		}
	}()
	return handler.Handler(ctx, payload)
}
func (c *Consumer) deserializePayload(handler *TopicHandler, data []byte) (interface{}, error) {
	payloadPtr := reflect.New(handler.PayloadType)
	payload := payloadPtr.Interface()
	if err := json.Unmarshal(data, payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	return payloadPtr.Elem().Interface(), nil
}
func (c *Consumer) sendToDeadLetterTopic(ctx context.Context, deadLetterTopic string, originalMessage *kgo.Record, processingError error) error {
	deadLetterPayload := map[string]interface{}{
		"originalTopic":     originalMessage.Topic,
		"originalPartition": originalMessage.Partition,
		"originalOffset":    originalMessage.Offset,
		"originalKey":       string(originalMessage.Key),
		"originalValue":     string(originalMessage.Value),
		"originalHeaders":   convertHeaders(originalMessage.Headers),
		"errorMessage":      processingError.Error(),
		"failedAt":          time.Now().UTC(),
		"consumer":          c.name,
	}
	message := &ProducerMessage{
		Key:   string(originalMessage.Key),
		Value: deadLetterPayload,
		Headers: map[string]interface{}{
			"original-topic":     originalMessage.Topic,
			"error-message":      processingError.Error(),
			"consumer":           c.name,
			"failed-at":          time.Now().UTC().Format(time.RFC3339),
		},
	}
	return c.kafkaService.Emit(ctx, deadLetterTopic, message)
}
func convertHeaders(headers []kgo.RecordHeader) map[string]string {
	result := make(map[string]string)
	for _, header := range headers {
		result[header.Key] = string(header.Value)
	}
	return result
}
func getStackTrace() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}
func (c *Consumer) GetTopics() []string {
	topics := make([]string, 0, len(c.handlers))
	for topic := range c.handlers {
		topics = append(topics, topic)
	}
	return topics
}
func (c *Consumer) GetHandlerInfo() map[string]HandlerInfo {
	info := make(map[string]HandlerInfo)
	for topic, handler := range c.handlers {
		info[topic] = HandlerInfo{
			Topic:       topic,
			PayloadType: handler.PayloadType.String(),
			Options:     *handler.Options,
		}
	}
	return info
}
type HandlerInfo struct {
	Topic       string           `json:"topic"`
	PayloadType string           `json:"payloadType"`
	Options     ConsumerOptions  `json:"options"`
}
type BaseConsumer struct {
	*Consumer
}
func NewBaseConsumer(name string, kafkaService *KafkaService) *BaseConsumer {
	return &BaseConsumer{
		Consumer: NewConsumer(name, kafkaService),
	}
}
func (bc *BaseConsumer) WithHandler(handler *TopicHandler) *BaseConsumer {
	bc.RegisterHandler(handler)
	return bc
}
func (bc *BaseConsumer) WithHandlers(handlers ...*TopicHandler) *BaseConsumer {
	for _, handler := range handlers {
		bc.RegisterHandler(handler)
	}
	return bc
}
func GetFunctionName(fn interface{}) string {
	name := runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
	parts := strings.Split(name, ".")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return name
}
