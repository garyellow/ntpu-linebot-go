// Package bot provides the handler interface and utilities for LINE bot modules.
// Each module (id, course, contact) implements the Handler interface to process
// user messages and postback events.
package bot

import (
	"context"

	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// Handler defines the interface that all bot modules must implement
// This provides a consistent API for webhook routing and message handling
type Handler interface {
	// CanHandle checks if this handler can process the given text message
	// Returns true if the handler recognizes keywords or patterns in the text
	CanHandle(text string) bool

	// HandleMessage processes a text message and returns LINE message responses
	// The context should be used for cancellation and timeout management
	// Returns a slice of LINE messages (max 5 messages per reply)
	HandleMessage(ctx context.Context, text string) []messaging_api.MessageInterface

	// HandlePostback processes a postback event (button clicks, carousel actions)
	// The data parameter contains the postback payload string (max 300 bytes per LINE API)
	//
	// Postback Format Convention:
	//   - Format: "module$action$param1$param2..." using $ as delimiter
	//   - Example: "course$detail$1131U1001" or "id$year$113"
	//   - Max 300 bytes per LINE API limit
	//   - No escaping mechanism for $ character (avoid in parameter values)
	//
	// Design Trade-off:
	//   Simple string concatenation vs JSON encoding
	//   - Current: Fast, simple, works for controlled data
	//   - Alternative: JSON (safer but adds overhead)
	//   For structured data with special characters, consider JSON encoding
	//
	// Returns a slice of LINE messages (max 5 messages per reply)
	HandlePostback(ctx context.Context, data string) []messaging_api.MessageInterface
}
