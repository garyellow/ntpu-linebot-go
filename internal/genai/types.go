// Package genai provides integration with Google's Generative AI APIs.
// This file contains shared types for NLU intent parsing.
package genai

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
