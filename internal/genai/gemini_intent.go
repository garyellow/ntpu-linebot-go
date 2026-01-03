// Package genai provides integration with LLM APIs (Gemini and Groq).
// This file contains the Gemini implementation of NLU intent parsing.
package genai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

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
		model = DefaultGeminiIntentModels[0]
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

// Parse analyzes the user input and returns a parsed intent.
// The model uses ANY mode, requiring it to always call a function (including direct_reply for non-query responses).
func (p *geminiIntentParser) Parse(ctx context.Context, text string) (*ParseResult, error) {
	if p == nil {
		return nil, errors.New("intent parser is nil")
	}

	// Configure generation with tools in ANY mode (forces function calling)
	config := &genai.GenerateContentConfig{
		Tools:             p.tools,
		SystemInstruction: genai.NewContentFromText(p.systemInst, genai.RoleUser),
		ToolConfig: &genai.ToolConfig{
			FunctionCallingConfig: &genai.FunctionCallingConfig{
				Mode: genai.FunctionCallingConfigModeAny, // Force function calling
			},
		},
		Temperature:     genai.Ptr[float32](0.1), // Low temperature for consistent classification
		MaxOutputTokens: 512,                     // Sufficient for direct_reply messages with clarification options
	}

	// Generate content with timing
	start := time.Now()
	result, err := p.client.Models.GenerateContent(
		ctx,
		p.model,
		genai.Text(text),
		config,
	)
	duration := time.Since(start)

	if err != nil {
		slog.WarnContext(ctx, "intent parsing API call failed",
			"provider", "gemini",
			"model", p.model,
			"input_length", len(text),
			"duration_ms", duration.Milliseconds(),
			"error", err)
		return nil, fmt.Errorf("generate content failed: %w", err)
	}

	// Parse the result
	parsedResult, parseErr := p.parseResult(result)

	// Log success with token usage
	if parseErr == nil && result.UsageMetadata != nil {
		slog.DebugContext(ctx, "intent parsing completed",
			"provider", "gemini",
			"model", p.model,
			"input_tokens", result.UsageMetadata.PromptTokenCount,
			"output_tokens", result.UsageMetadata.CandidatesTokenCount,
			"total_tokens", result.UsageMetadata.TotalTokenCount,
			"duration_ms", duration.Milliseconds(),
			"function_name", func() string {
				if parsedResult != nil {
					return parsedResult.FunctionName
				}
				return ""
			}())
	}

	return parsedResult, parseErr
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

	// Check each part for function call (ANY mode forces function calling)
	for _, part := range candidate.Content.Parts {
		if part.FunctionCall != nil {
			return p.parseFunctionCall(part.FunctionCall)
		}
	}

	// In ANY mode, model should always return a function call
	// If we get here, something unexpected happened
	return nil, errors.New("no function call in response (expected with ANY mode)")
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
	// Note: genai.Client does not require explicit cleanup in current SDK version
	return nil
}
