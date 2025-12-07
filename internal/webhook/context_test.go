package webhook

import (
	"context"
	"testing"
)

func TestWebhookContextBackwardCompatibility(t *testing.T) {
	t.Run("GetChatID and GetUserID work with standard context", func(t *testing.T) {
		ctx := context.Background()

		// These functions now use internal/context package
		// and should return empty strings for context without values
		chatID := GetChatID(ctx)
		if chatID != "" {
			t.Errorf("expected empty string for chatID, got %s", chatID)
		}

		userID := GetUserID(ctx)
		if userID != "" {
			t.Errorf("expected empty string for userID, got %s", userID)
		}
	})
}
