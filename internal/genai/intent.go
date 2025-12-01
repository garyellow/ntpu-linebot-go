// Package genai provides integration with Google's Generative AI APIs.
// This file contains the NLU intent parser using Gemini function calling.
package genai

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/genai"
)

const (
	// IntentParserModel is the model used for intent parsing
	// Using gemini-2.5-flash-lite for fast, cost-effective intent classification
	IntentParserModel = "gemini-2.5-flash-lite"

	// IntentParserTimeout is the timeout for intent parsing requests
	IntentParserTimeout = 10 * time.Second
)

// IntentParser provides NLU intent parsing using Gemini function calling.
// It implements IntentParserInterface defined in types.go.
type IntentParser struct {
	client     *genai.Client
	model      string
	tools      []*genai.Tool
	systemInst string
}

// NewIntentParser creates a new intent parser.
// Returns nil if apiKey is empty (NLU disabled).
func NewIntentParser(ctx context.Context, apiKey string) (*IntentParser, error) {
	if apiKey == "" {
		return nil, nil // NLU disabled
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	// Build function declarations
	funcDecls := BuildIntentFunctions()

	return &IntentParser{
		client: client,
		model:  IntentParserModel,
		tools: []*genai.Tool{{
			FunctionDeclarations: funcDecls,
		}},
		systemInst: IntentParserSystemPrompt,
	}, nil
}

// Parse analyzes the user input and returns a parsed intent or clarification text.
// The model uses AUTO mode (default), allowing it to either call a function or return text.
func (p *IntentParser) Parse(ctx context.Context, text string) (*ParseResult, error) {
	if p == nil {
		return nil, fmt.Errorf("intent parser is nil")
	}

	// Create timeout context
	// Note: If parent context has less time remaining than IntentParserTimeout,
	// the child context will be limited by the parent's deadline.
	// This is expected behavior as we inherit the webhook timeout constraint.
	ctx, cancel := context.WithTimeout(ctx, IntentParserTimeout)
	defer cancel()

	// Configure generation with tools (AUTO mode is default)
	config := &genai.GenerateContentConfig{
		Tools:             p.tools,
		SystemInstruction: genai.NewContentFromText(p.systemInst, genai.RoleUser),
		Temperature:       genai.Ptr[float32](0.1), // Low temperature for consistent classification
		MaxOutputTokens:   256,                     // Intent parsing doesn't need long responses
	}

	// Generate content
	result, err := p.client.Models.GenerateContent(
		ctx,
		p.model,
		genai.Text(text),
		config,
	)
	if err != nil {
		return nil, fmt.Errorf("generate content failed: %w", err)
	}

	// Parse the result
	return p.parseResult(result)
}

// parseResult extracts intent information from the generation result.
func (p *IntentParser) parseResult(result *genai.GenerateContentResponse) (*ParseResult, error) {
	if result == nil || len(result.Candidates) == 0 {
		return nil, fmt.Errorf("empty response from model")
	}

	candidate := result.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return nil, fmt.Errorf("no content in response")
	}

	// Check each part for function call or text
	for _, part := range candidate.Content.Parts {
		// Check for function call
		if part.FunctionCall != nil {
			return p.parseFunctionCall(part.FunctionCall)
		}

		// Check for text response (clarification)
		if part.Text != "" {
			return &ParseResult{
				ClarificationText: part.Text,
			}, nil
		}
	}

	return nil, fmt.Errorf("no function call or text in response")
}

// parseFunctionCall extracts intent and parameters from a function call.
func (p *IntentParser) parseFunctionCall(fc *genai.FunctionCall) (*ParseResult, error) {
	funcName := fc.Name

	// Look up module and intent
	moduleIntent, ok := IntentModuleMap[funcName]
	if !ok {
		return nil, fmt.Errorf("unknown function: %s", funcName)
	}

	// Extract parameters
	params := make(map[string]string)
	if paramKey, hasParam := ParamKeyMap[funcName]; hasParam {
		value, exists := fc.Args[paramKey]
		if !exists {
			// Required parameter is missing from function call
			return nil, fmt.Errorf("missing required parameter %q for function %q", paramKey, funcName)
		}
		strVal, ok := value.(string)
		if !ok {
			// Parameter exists but is not a string type
			return nil, fmt.Errorf("parameter %q for function %q is not a string (got %T)", paramKey, funcName, value)
		}
		params[paramKey] = strVal
	}

	return &ParseResult{
		Module:       moduleIntent[0],
		Intent:       moduleIntent[1],
		Params:       params,
		FunctionName: funcName,
	}, nil
}

// IsEnabled returns true if the intent parser is enabled.
func (p *IntentParser) IsEnabled() bool {
	return p != nil && p.client != nil
}

// Close releases resources held by the intent parser.
// Note: genai.Client is stateless and doesn't require explicit closure,
// but this method is provided for interface compatibility.
func (p *IntentParser) Close() error {
	// genai.Client doesn't have a Close method as it's stateless
	return nil
}
