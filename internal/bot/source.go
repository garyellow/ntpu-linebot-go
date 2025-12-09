package bot

import "github.com/line/line-bot-sdk-go/v8/linebot/webhook"

// GetChatID extracts the chat ID from a LINE source.
// Returns user ID for personal chats, group ID for groups, room ID for rooms.
// Returns empty string if source type is unknown.
func GetChatID(source webhook.SourceInterface) string {
	switch s := source.(type) {
	case webhook.UserSource:
		return s.UserId
	case webhook.GroupSource:
		return s.GroupId
	case webhook.RoomSource:
		return s.RoomId
	}
	return ""
}

// GetUserID extracts the user ID from a LINE source.
// Returns the user ID regardless of chat type (personal, group, or room).
// Returns empty string if source type is unknown or user ID is not available.
func GetUserID(source webhook.SourceInterface) string {
	switch s := source.(type) {
	case webhook.UserSource:
		return s.UserId
	case webhook.GroupSource:
		return s.UserId
	case webhook.RoomSource:
		return s.UserId
	}
	return ""
}

// IsPersonalChat checks if the source is a personal (1-on-1) chat.
// Returns true for UserSource, false for GroupSource and RoomSource.
func IsPersonalChat(source webhook.SourceInterface) bool {
	_, ok := source.(webhook.UserSource)
	return ok
}
