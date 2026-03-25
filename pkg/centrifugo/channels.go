package centrifugo

// ChannelFromRoomID converts a room ID (user:{id} or topic:{topic}) to a
// Centrifugo channel name under the "notifications" namespace.
// Example: "user:abc" → "notifications:user:abc"
func ChannelFromRoomID(roomID string) string {
	return "notifications:" + roomID
}
