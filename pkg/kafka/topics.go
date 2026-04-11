package kafka

// Topic base names for the chat service (without environment prefix).
// Use GetTopicWithEnv to get the fully-qualified name at runtime.
const (
	TopicMessagesInbound = "chat.messages.inbound"
	TopicMessagesSent    = "chat.messages.sent"
	TopicMessagesUpdated = "chat.messages.updated"
	TopicMessagesDeleted = "chat.messages.deleted"
	TopicReadReceipts    = "chat.read_receipts"
	TopicReactions       = "chat.reactions"
)

// GetTopicWithEnv returns the fully-qualified topic name.
// If env is empty, the base topic name is returned unchanged.
func GetTopicWithEnv(env, topic string) string {
	if env == "" {
		return topic
	}
	return env + "." + topic
}

// GetAllTopics returns all topics managed by this service with the env prefix applied.
func GetAllTopics(env string) []string {
	base := []string{
		TopicMessagesInbound,
		TopicMessagesSent,
		TopicMessagesUpdated,
		TopicMessagesDeleted,
		TopicReadReceipts,
		TopicReactions,
	}
	topics := make([]string, len(base))
	for i, t := range base {
		topics[i] = GetTopicWithEnv(env, t)
	}
	return topics
}
