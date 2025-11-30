// Package genai provides integration with Google's Generative AI APIs.
// This file contains shared types for NLU intent parsing.
package genai

import "context"

// ParseResult represents the result of intent parsing.
type ParseResult struct {
	// Module is the target module (course, id, contact, help)
	Module string

	// Intent is the specific intent within the module
	Intent string

	// Params contains the extracted parameters
	Params map[string]string

	// ClarificationText is set when the model returns text instead of a function call
	// This usually means the model needs more information or the query is out of scope
	ClarificationText string

	// FunctionName is the raw function name from the model (for debugging)
	FunctionName string
}

// IntentParserInterface defines the interface for NLU intent parsing.
// This interface allows components to use the intent parser without
// directly depending on the full implementation.
type IntentParserInterface interface {
	// Parse analyzes the user input and returns a parsed intent.
	Parse(ctx context.Context, text string) (*ParseResult, error)

	// IsEnabled returns true if the parser is enabled.
	IsEnabled() bool
}
