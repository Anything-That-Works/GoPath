package cache

import (
	"fmt"
	"time"
)

const (
	TTLUserProfile         = 5 * time.Minute
	TTLConversationsList   = 2 * time.Minute
	TTLConversationMembers = 5 * time.Minute
)

func KeyUserProfile(userID string) string {
	return fmt.Sprintf("user:profile:%s", userID)
}

func KeyConversationsList(userID string, page, limit int32) string {
	return fmt.Sprintf("conversations:list:%s:%d:%d", userID, page, limit)
}

func KeyConversationMembers(conversationID string) string {
	return fmt.Sprintf("conversation:members:%s", conversationID)
}
