// Package genai provides integration with LLM APIs (Gemini and Groq).
// This file contains the Gemini implementation of NLU intent parsing.
package genai

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/genai"
)

// geminiIntentParser provides NLU intent parsing using Gemini function calling.
// It implements the IntentParser interface.
type geminiIntentParser struct {
	client     *genai.Client
	model      string
	tools      []*genai.Tool
	systemInst string
}

// newGeminiIntentParser creates a new Gemini-based intent parser.
// Returns nil if apiKey is empty (NLU disabled).
func newGeminiIntentParser(ctx context.Context, apiKey, model string) (*geminiIntentParser, error) {
	if apiKey == "" {
		return nil, nil //nolint:nilnil // Intentional: NLU disabled when no API key
	}

	if model == "" {
		model = DefaultGeminiIntentModel
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	// Build function declarations
	funcDecls := BuildIntentFunctions()

	return &geminiIntentParser{
		client: client,
		model:  model,
		tools: []*genai.Tool{{
			FunctionDeclarations: funcDecls,
		}},
		systemInst: IntentParserSystemPrompt,
	}, nil
}

// Parse analyzes the user input and returns a parsed intent or clarification text.
// The model uses AUTO mode (default), allowing it to either call a function or return text.
func (p *geminiIntentParser) Parse(ctx context.Context, text string) (*ParseResult, error) {
	if p == nil {
		return nil, errors.New("intent parser is nil")
	}

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
func (p *geminiIntentParser) parseResult(result *genai.GenerateContentResponse) (*ParseResult, error) {
	if result == nil || len(result.Candidates) == 0 {
		return nil, errors.New("empty response from model")
	}

	candidate := result.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return nil, errors.New("no content in response")
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

	return nil, errors.New("no function call or text in response")
}

// parseFunctionCall extracts intent and parameters from a function call.
func (p *geminiIntentParser) parseFunctionCall(fc *genai.FunctionCall) (*ParseResult, error) {
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
func (p *geminiIntentParser) IsEnabled() bool {
	return p != nil && p.client != nil
}

// Provider returns the provider type for this parser.
func (p *geminiIntentParser) Provider() Provider {
	return ProviderGemini
}

// Close releases resources held by the geminiIntentParser.
// Safe to call on nil receiver.
func (p *geminiIntentParser) Close() error {
	if p == nil {
		return nil
	}
	// Future: Add client.Close() when SDK supports it
	return nil
}
