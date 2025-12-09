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
	// The data parameter contains the PostbackData structure.
	//
	// Postback Format:
	//   - String format: "module:action$param1$param2"
	//   - Max 300 bytes per LINE API limit
	//   - Use bot.PostbackSplitChar ("$") as parameter separator
	//
	// Example:
	//   fmt.Sprintf("course:detail%s%s", bot.PostbackSplitChar, courseUID)
	//
	// Returns a slice of LINE messages (max 5 messages per reply per LINE API).
	HandlePostback(ctx context.Context, data string) []messaging_api.MessageInterface
}

// NLUHandler defines the interface for modules that support NLU intent dispatching.
type NLUHandler interface {
	Handler
	// DispatchIntent dispatches a parsed NLU intent to the handler.
	DispatchIntent(ctx context.Context, intent string, params map[string]string) ([]messaging_api.MessageInterface, error)
}
