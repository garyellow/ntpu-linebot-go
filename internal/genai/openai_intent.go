// Package genai provides integration with LLM APIs (Gemini, Groq, and Cerebras).
// This file contains the unified OpenAI-compatible implementation of NLU intent parsing.
// It works with any OpenAI-compatible provider (Groq, Cerebras) via custom BaseURL.
package genai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// openaiIntentParser provides NLU intent parsing using OpenAI-compatible API.
// It implements the IntentParser interface.
// Works with Groq, Cerebras, and other OpenAI-compatible providers.
type openaiIntentParser struct {
	client     openai.Client
	model      string
	tools      []openai.ChatCompletionToolUnionParam
	systemInst string
	provider   Provider
}

// newOpenAIIntentParser creates a new OpenAI-compatible intent parser.
// Returns nil if apiKey is empty (NLU disabled).
//
// Parameters:
//   - provider: The provider type (ProviderGroq, ProviderCerebras, ProviderOpenAI)
//   - apiKey: The API key for the provider
//   - model: The model name to use (uses provider defaults if empty)
//   - endpoint: Custom base URL for ProviderOpenAI (ignored for other providers)
func newOpenAIIntentParser(_ context.Context, provider Provider, apiKey, model, endpoint string) (*openaiIntentParser, error) {
	if apiKey == "" {
		return nil, nil //nolint:nilnil // Intentional: NLU disabled when no API key
	}

	// Get the base URL for the provider
	var baseURL string
	if endpoint != "" {
		// Use custom endpoint (ProviderOpenAI)
		baseURL = endpoint
	} else {
		// Use predefined endpoint from ProviderEndpoint map
		var ok bool
		baseURL, ok = ProviderEndpoint[provider]
		if !ok {
			return nil, fmt.Errorf("unsupported OpenAI-compatible provider: %s", provider)
		}
	}

	// Use default model if not specified
	if model == "" {
		switch provider {
		case ProviderGroq:
			model = DefaultGroqIntentModels[0]
		case ProviderCerebras:
			model = DefaultCerebrasIntentModels[0]
		case ProviderOpenAI:
			// OpenAI-compatible requires explicit model
			return nil, fmt.Errorf("model is required for OpenAI-compatible provider")
		default:
			return nil, fmt.Errorf("no default model for provider: %s", provider)
		}
	}

	// Create client with custom base URL
	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(apiKey),
	)

	// Build tools from function declarations
	tools := buildOpenAITools()

	return &openaiIntentParser{
		client:     client,
		model:      model,
		tools:      tools,
		systemInst: IntentParserSystemPrompt,
		provider:   provider,
	}, nil
}

// buildOpenAITools converts our function declarations to OpenAI v3 tool format.
// OpenAI API uses lowercase JSON Schema types per Draft 2020-12 spec.
func buildOpenAITools() []openai.ChatCompletionToolUnionParam {
	funcDecls := BuildIntentFunctions()
	result := make([]openai.ChatCompletionToolUnionParam, 0, len(funcDecls))

	for _, fd := range funcDecls {
		// Build properties map
		properties := make(map[string]any)
		required := make([]string, 0)

		for name, schema := range fd.Parameters.Properties {
			// IMPORTANT: Convert genai.Type* constants to lowercase for OpenAI API
			// genai.TypeString = "STRING" → "string" (required by JSON Schema spec)
			schemaType := strings.ToLower(string(schema.Type))
			properties[name] = map[string]string{
				"type":        schemaType,
				"description": schema.Description,
			}

			// All parameters with descriptions are required
			if schema.Description != "" {
				required = append(required, name)
			}
		}

		// v3 API uses ChatCompletionFunctionTool helper
		tool := openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        fd.Name,
			Description: openai.String(fd.Description),
			Parameters: openai.FunctionParameters{
				"type":       "object",
				"properties": properties,
				"required":   required,
			},
		})
		result = append(result, tool)
	}

	return result
}

// Parse analyzes the user input and returns a parsed intent.
// The model uses required mode (forces function calling, including direct_reply for non-query responses).
func (p *openaiIntentParser) Parse(ctx context.Context, text string) (*ParseResult, error) {
	if p == nil {
		return nil, errors.New("intent parser is nil")
	}

	// Build chat completion request with tools in required mode (forces function calling).
	// ChatCompletionToolChoiceOptionAutoRequired ensures the model must call a function.
	params := openai.ChatCompletionNewParams{
		Model: p.model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(p.systemInst),
			openai.UserMessage(text),
		},
		Tools: p.tools,
		ToolChoice: openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.String(string(openai.ChatCompletionToolChoiceOptionAutoRequired)),
		},
		Temperature: openai.Float(0.1), // Low temperature for consistent classification
		MaxTokens:   openai.Int(512),   // Sufficient for direct_reply messages with clarification options
	}

	// Execute request with timing
	start := time.Now()
	resp, err := p.client.Chat.Completions.New(ctx, params)
	duration := time.Since(start)

	if err != nil {
		slog.WarnContext(ctx, "Intent parsing API call failed",
			"provider", p.provider,
			"model", p.model,
			"input_length", len(text),
			"duration_ms", duration.Milliseconds(),
			"error", err)
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}

	// Parse the result
	parsedResult, parseErr := p.parseResult(resp)

	// Log success with token usage
	if parseErr == nil && resp.Usage.TotalTokens > 0 {
		slog.DebugContext(ctx, "Intent parsing completed",
			"provider", p.provider,
			"model", p.model,
			"input_tokens", resp.Usage.PromptTokens,
			"output_tokens", resp.Usage.CompletionTokens,
			"total_tokens", resp.Usage.TotalTokens,
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

// parseResult extracts intent information from the OpenAI response.
func (p *openaiIntentParser) parseResult(resp *openai.ChatCompletion) (*ParseResult, error) {
	if resp == nil || len(resp.Choices) == 0 {
		return nil, errors.New("empty response from model")
	}

	choice := resp.Choices[0]

	// Check for tool calls (required mode forces function calling)
	if len(choice.Message.ToolCalls) > 0 {
		return p.parseToolCall(choice.Message.ToolCalls[0])
	}

	// In required mode, model should always return a tool call
	// If we get here, something unexpected happened
	return nil, errors.New("no tool call in response (expected with required mode)")
}

// parseToolCall extracts intent and parameters from an OpenAI tool call.
// Iterates through all parameter keys defined in ParamKeysMap to extract values.
// v3 API: ChatCompletionMessageToolCallUnion is the union type for tool calls.
func (p *openaiIntentParser) parseToolCall(tc openai.ChatCompletionMessageToolCallUnion) (*ParseResult, error) {
	if tc.Type != "function" {
		return nil, fmt.Errorf("unexpected tool type: %s", tc.Type)
	}

	funcName := tc.Function.Name

	// Look up module and intent
	moduleIntent, ok := IntentModuleMap[funcName]
	if !ok {
		return nil, fmt.Errorf("unknown function: %s", funcName)
	}

	// Parse JSON arguments once
	var args map[string]any
	if tc.Function.Arguments != "" {
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			return nil, fmt.Errorf("failed to parse function arguments: %w", err)
		}
	}

	// Extract all parameters defined for this function
	params := make(map[string]string)
	if paramKeys, hasParams := ParamKeysMap[funcName]; hasParams {
		for _, paramKey := range paramKeys {
			value, exists := args[paramKey]
			if !exists {
				// Parameter not provided by model - handler will validate if required
				continue
			}
			strVal, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("parameter %q for function %q is not a string (got %T)", paramKey, funcName, value)
			}
			params[paramKey] = strVal
		}
	}

	return &ParseResult{
		Module:       moduleIntent[0],
		Intent:       moduleIntent[1],
		Params:       params,
		FunctionName: funcName,
	}, nil
}

// IsEnabled returns true if the intent parser is enabled.
func (p *openaiIntentParser) IsEnabled() bool {
	return p != nil
}

// Provider returns the provider type for this parser.
func (p *openaiIntentParser) Provider() Provider {
	if p == nil {
		return ""
	}
	return p.provider
}

// Close releases resources held by the openaiIntentParser.
// Safe to call on nil receiver.
func (p *openaiIntentParser) Close() error {
	if p == nil {
		return nil
	}
	// openai-go client doesn't require cleanup
	return nil
}
