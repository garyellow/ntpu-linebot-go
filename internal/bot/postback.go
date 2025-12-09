package bot

import (
	"errors"
	"fmt"
	"strings"
)

// PostbackData represents structured postback payload.
// Format: "module:action$param1$param2" (legacy string-based format)
// This format is used throughout the bot for postback actions.
type PostbackData struct {
	Module string            // Module identifier (e.g., "course", "id", "contact")
	Action string            // Action identifier (e.g., "detail", "search")
	Params map[string]string // Parameters (indexed keys: "p0", "p1", etc.)
}

// decodePostback parses the string-based postback format.
// Format: "module:action$param1$param2"
func decodePostback(data string) (*PostbackData, error) {
	parts := strings.SplitN(data, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid postback format: missing ':' separator")
	}

	module := parts[0]
	remainder := parts[1]

	actionAndParams := strings.Split(remainder, PostbackSplitChar)
	if len(actionAndParams) == 0 {
		return nil, fmt.Errorf("invalid postback format: missing action")
	}

	action := actionAndParams[0]
	params := make(map[string]string)

	// Use indexed keys for parameters
	for i, param := range actionAndParams[1:] {
		params[fmt.Sprintf("p%d", i)] = param
	}

	return &PostbackData{
		Module: module,
		Action: action,
		Params: params,
	}, nil
}

// ParsePostback parses postback data string into structured PostbackData.
// Returns error if the format is invalid.
func ParsePostback(data string) (*PostbackData, error) {
	pb, err := decodePostback(data)
	if err != nil {
		return nil, fmt.Errorf("invalid postback format: %w", errors.Join(err))
	}
	return pb, nil
}
