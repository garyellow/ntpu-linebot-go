package bot

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// PostbackData represents structured postback payload.
// Using JSON encoding instead of string concatenation improves:
// - Type safety: compile-time validation
// - Escaping: automatic handling of special characters
// - Maintainability: clear structure
type PostbackData struct {
	Module string            `json:"m"`           // Module identifier (e.g., "course", "id")
	Action string            `json:"a"`           // Action identifier (e.g., "detail", "search")
	Params map[string]string `json:"p,omitempty"` // Optional parameters
}

// EncodePostback encodes postback data to JSON string.
// Returns error if encoding fails or result exceeds LINE API limit (300 bytes).
func EncodePostback(module, action string, params map[string]string) (string, error) {
	data := PostbackData{
		Module: module,
		Action: action,
		Params: params,
	}

	encoded, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("encode postback: %w", err)
	}

	// LINE API限制: postback data 最多 300 bytes
	if len(encoded) > 300 {
		return "", fmt.Errorf("postback data exceeds 300 bytes: %d", len(encoded))
	}

	return string(encoded), nil
}

// DecodePostback decodes JSON postback data.
func DecodePostback(data string) (*PostbackData, error) {
	var pb PostbackData
	if err := json.Unmarshal([]byte(data), &pb); err != nil {
		return nil, fmt.Errorf("decode postback: %w", err)
	}
	return &pb, nil
}

// LegacyDecodePostback handles old string-based format for backward compatibility.
// Format: "module:action$param1$param2"
func LegacyDecodePostback(data string) (*PostbackData, error) {
	parts := strings.SplitN(data, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid legacy format: missing ':' separator")
	}

	module := parts[0]
	remainder := parts[1]

	actionAndParams := strings.Split(remainder, "$")
	if len(actionAndParams) == 0 {
		return nil, fmt.Errorf("invalid legacy format: missing action")
	}

	action := actionAndParams[0]
	params := make(map[string]string)

	// Legacy format doesn't have named parameters, use indexed keys
	for i, param := range actionAndParams[1:] {
		params[fmt.Sprintf("p%d", i)] = param
	}

	return &PostbackData{
		Module: module,
		Action: action,
		Params: params,
	}, nil
}

// ParsePostback attempts JSON decoding first, falls back to legacy format.
func ParsePostback(data string) (*PostbackData, error) {
	// Try JSON first
	pb, err := DecodePostback(data)
	if err == nil {
		return pb, nil
	}

	// Fallback to legacy format
	pb, legacyErr := LegacyDecodePostback(data)
	if legacyErr == nil {
		return pb, nil
	}

	return nil, fmt.Errorf("invalid postback format: %w", errors.Join(err, legacyErr))
}
