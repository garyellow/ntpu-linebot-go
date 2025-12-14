// Package genai provides integration with LLM APIs (Gemini and Groq).
// This file contains the Groq implementation of NLU intent parsing.
package genai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/conneroisu/groq-go"
	"github.com/conneroisu/groq-go/pkg/tools"
)

// groqIntentParser provides NLU intent parsing using Groq function calling.
// It implements the IntentParser interface.
type groqIntentParser struct {
	client     *groq.Client
	model      groq.ChatModel
	tools      []tools.Tool
	systemInst string
}

// newGroqIntentParser creates a new Groq-based intent parser.
// Returns nil if apiKey is empty (NLU disabled).
func newGroqIntentParser(_ context.Context, apiKey, model string) (*groqIntentParser, error) {
	if apiKey == "" {
		return nil, nil //nolint:nilnil // Intentional: NLU disabled when no API key
	}

	if model == "" {
		model = DefaultGroqIntentModel
	}

	client, err := groq.NewClient(apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create groq client: %w", err)
	}

	// Build function declarations for Groq
	groqTools := buildGroqTools()

	return &groqIntentParser{
		client:     client,
		model:      groq.ChatModel(model),
		tools:      groqTools,
		systemInst: IntentParserSystemPrompt,
	}, nil
}

// buildGroqTools converts our function declarations to Groq tool format
func buildGroqTools() []tools.Tool {
	funcDecls := BuildIntentFunctions()
	result := make([]tools.Tool, 0, len(funcDecls))

	for _, fd := range funcDecls {
		// Convert properties to map[string]tools.PropertyDefinition format
		properties := make(map[string]tools.PropertyDefinition)
		required := make([]string, 0)

		for name, schema := range fd.Parameters.Properties {
			properties[name] = tools.PropertyDefinition{
				Type:        string(schema.Type),
				Description: schema.Description,
			}

			// All parameters with descriptions are required
			if schema.Description != "" {
				required = append(required, name)
			}
		}

		tool := tools.Tool{
			Type: tools.ToolTypeFunction,
			Function: tools.FunctionDefinition{
				Name:        fd.Name,
				Description: fd.Description,
				Parameters: tools.FunctionParameters{
					Type:       "object",
					Properties: properties,
					Required:   required,
				},
			},
		}
		result = append(result, tool)
	}

	return result
}

// Parse analyzes the user input and returns a parsed intent or clarification text.
func (p *groqIntentParser) Parse(ctx context.Context, text string) (*ParseResult, error) {
	if p == nil {
		return nil, errors.New("intent parser is nil")
	}

	// Build chat completion request with tools
	req := groq.ChatCompletionRequest{
		Model: p.model,
		Messages: []groq.ChatCompletionMessage{
			{
				Role:    groq.RoleSystem,
				Content: p.systemInst,
			},
			{
				Role:    groq.RoleUser,
				Content: text,
			},
		},
		Tools:       p.tools,
		ToolChoice:  "auto", // Let the model decide
		Temperature: 0.1,    // Low temperature for consistent classification
		MaxTokens:   256,
	}

	// Execute request with timing
	start := time.Now()
	resp, err := p.client.ChatCompletion(ctx, req)
	duration := time.Since(start)

	if err != nil {
		slog.WarnContext(ctx, "intent parsing API call failed",
			"provider", "groq",
			"model", p.model,
			"input_length", len(text),
			"duration_ms", duration.Milliseconds(),
			"error", err)
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}

	// Parse the result
	parsedResult, parseErr := p.parseResult(&resp)

	// Log success with token usage
	if parseErr == nil && resp.Usage.TotalTokens > 0 {
		slog.InfoContext(ctx, "intent parsing completed",
			"provider", "groq",
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

// parseResult extracts intent information from the Groq response.
func (p *groqIntentParser) parseResult(resp *groq.ChatCompletionResponse) (*ParseResult, error) {
	if resp == nil || len(resp.Choices) == 0 {
		return nil, errors.New("empty response from model")
	}

	choice := resp.Choices[0]

	// Check for tool calls (function calling)
	if len(choice.Message.ToolCalls) > 0 {
		return p.parseToolCall(&choice.Message.ToolCalls[0])
	}

	// Check for text response (clarification)
	if choice.Message.Content != "" {
		return &ParseResult{
			ClarificationText: choice.Message.Content,
		}, nil
	}

	return nil, errors.New("no tool call or text in response")
}

// parseToolCall extracts intent and parameters from a Groq tool call.
func (p *groqIntentParser) parseToolCall(tc *tools.ToolCall) (*ParseResult, error) {
	if tc.Type != string(tools.ToolTypeFunction) {
		return nil, fmt.Errorf("unexpected tool type: %s", tc.Type)
	}

	funcName := tc.Function.Name

	// Look up module and intent
	moduleIntent, ok := IntentModuleMap[funcName]
	if !ok {
		return nil, fmt.Errorf("unknown function: %s", funcName)
	}

	// Parse JSON arguments
	params := make(map[string]string)
	if paramKey, hasParam := ParamKeyMap[funcName]; hasParam {
		// Groq returns arguments as JSON string
		var args map[string]any
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			return nil, fmt.Errorf("failed to parse function arguments: %w", err)
		}

		value, exists := args[paramKey]
		if !exists {
			return nil, fmt.Errorf("missing required parameter %q for function %q", paramKey, funcName)
		}

		strVal, ok := value.(string)
		if !ok {
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
func (p *groqIntentParser) IsEnabled() bool {
	return p != nil && p.client != nil
}

// Provider returns the provider type for this parser.
func (p *groqIntentParser) Provider() Provider {
	return ProviderGroq
}

// Close releases resources held by the groqIntentParser.
// Safe to call on nil receiver.
func (p *groqIntentParser) Close() error {
	if p == nil {
		return nil
	}
	// groq-go client doesn't require cleanup
	return nil
}
