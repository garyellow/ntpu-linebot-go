package bot

import (
	"errors"
	"strings"
)

// PostbackData represents structured postback payload.
// Format: "module:action$param1$param2"
type PostbackData struct {
	Module string   // Module identifier (e.g., "course", "id", "contact")
	Action string   // Action identifier (e.g., "detail", "search")
	Params []string // Parameters in order
}

// ParsePostback parses postback data string into structured PostbackData.
// Format: "module:action$param1$param2"
// Returns error if the format is invalid.
func ParsePostback(data string) (*PostbackData, error) {
	parts := strings.SplitN(data, ":", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid postback format: missing ':' separator")
	}

	module := parts[0]
	remainder := parts[1]

	actionAndParams := strings.Split(remainder, PostbackSplitChar)
	if len(actionAndParams) == 0 {
		return nil, errors.New("invalid postback format: missing action")
	}

	return &PostbackData{
		Module: module,
		Action: actionAndParams[0],
		Params: actionAndParams[1:],
	}, nil
}
