package redis

import "time"

const (
	KeyNotificationCount = "notification_count"
	KeyUserPreference    = "user_preference"
)

const (
	DefaultCacheTTL = 24 * time.Hour
)
