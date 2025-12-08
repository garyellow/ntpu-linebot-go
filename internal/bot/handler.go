// Package bot provides the handler interface and utilities for LINE bot modules.
// Each module (id, course, contact) implements the Handler interface to process
// user messages and postback events.
package bot

import (
	"context"

	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// Handler defines the interface that all bot modules must implement.
// This provides a consistent API for webhook routing and message handling.
type Handler interface {
	// Name returns the unique module identifier (e.g., "id", "course", "contact").
	// This name is used for postback routing and logging.
	Name() string

	// CanHandle checks if this handler can process the given text message.
	// Returns true if the handler recognizes keywords or patterns in the text.
	CanHandle(text string) bool

	// HandleMessage processes a text message and returns LINE message responses.
	// The context should be used for cancellation and timeout management.
	// Returns a slice of LINE messages (max 5 messages per reply per LINE API).
	HandleMessage(ctx context.Context, text string) []messaging_api.MessageInterface

	// HandlePostback processes a postback event (button clicks, carousel actions).
	// The data parameter contains the JSON-encoded PostbackData structure.
	//
	// Postback Format:
	//   - JSON structure: {"m":"module","a":"action","p":{"key":"value"}}
	//   - Max 300 bytes per LINE API limit
	//   - Automatic escaping of special characters via JSON encoding
	//
	// Example:
	//   data, _ := bot.EncodePostback("course", "detail", map[string]string{"uid": "1131U1001"})
	//
	// Returns a slice of LINE messages (max 5 messages per reply per LINE API).
	HandlePostback(ctx context.Context, data string) []messaging_api.MessageInterface
}
