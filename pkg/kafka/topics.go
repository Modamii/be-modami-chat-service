package kafka

import (
	config "github.com/techinsight/be-techinsights-notification-service/configs"
)

type KafkaTopics struct {
	Article struct {
		Created string
		Updated string
		Deleted string
		Viewed  string
		Liked   string
		Unliked string
		Shared  string
	}
	User struct {
		Registered string
		Updated    string
	}
	Comment struct {
		Created string
		Deleted string
		Liked   string
		Unliked string
	}
}

func GetKafkaTopics() *KafkaTopics {
	return &KafkaTopics{
		Article: struct {
			Created string
			Updated string
			Deleted string
			Viewed  string
			Liked   string
			Unliked string
			Shared  string
		}{
			Created: "techinsight.article.created",
			Updated: "techinsight.article.updated",
			Deleted: "techinsight.article.deleted",
			Viewed:  "techinsight.article.viewed",
			Liked:   "techinsight.article.liked",
			Unliked: "techinsight.article.unliked",
			Shared:  "techinsight.article.shared",
		},
		User: struct {
			Registered string
			Updated    string
		}{
			Registered: "techinsight.user.registered",
			Updated:    "techinsight.user.updated",
		},
		Comment: struct {
			Created string
			Deleted string
			Liked   string
			Unliked string
		}{
			Created: "techinsight.comment.created",
			Deleted: "techinsight.comment.deleted",
			Liked:   "techinsight.comment.liked",
			Unliked: "techinsight.comment.unliked",
		},
	}
}

func GetTopicWithEnv(cfg *config.Config, topic string) string {
	env := cfg.App.Environment
	if env == "" {
		env = "local"
	}
	return env + "." + topic
}

func GetAllTopics() []string {
	topics := GetKafkaTopics()
	return []string{
		topics.Article.Created,
		topics.Article.Updated,
		topics.Article.Deleted,
		topics.Article.Viewed,
		topics.Article.Liked,
		topics.Article.Unliked,
		topics.Article.Shared,
		topics.User.Registered,
		topics.User.Updated,
		topics.Comment.Created,
		topics.Comment.Deleted,
		topics.Comment.Liked,
		topics.Comment.Unliked,
	}
}

func TopicExists(topic string) bool {
	allTopics := GetAllTopics()
	for _, t := range allTopics {
		if t == topic {
			return true
		}
	}
	return false
}
