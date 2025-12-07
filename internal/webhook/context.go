// Package webhook re-exports context utilities from internal/context.
// This maintains backward compatibility while centralizing context management.
package webhook

import "github.com/garyellow/ntpu-linebot-go/internal/context"

// Re-export context functions for backward compatibility.
var (
	GetChatID = context.GetChatID
	GetUserID = context.GetUserID
)
