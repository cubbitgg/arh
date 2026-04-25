package llm

import (
	"context"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// OpenAIClient wraps the OpenAI SDK.
type OpenAIClient struct {
	client openai.Client
	model  string
}

// NewOpenAIClient creates an OpenAI-backed LLM client.
func NewOpenAIClient(apiKey, model string) *OpenAIClient {
	if model == "" {
		model = "gpt-4o"
	}
	return &OpenAIClient{
		client: openai.NewClient(option.WithAPIKey(apiKey)),
		model:  model,
	}
}

// Complete sends systemPrompt + userPrompt to OpenAI and returns the response text.
func (o *OpenAIClient) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModel(o.model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
		MaxTokens: openai.Int(4096),
	})
	if err != nil {
		return "", fmt.Errorf("openai API call: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("openai returned no choices")
	}
	return resp.Choices[0].Message.Content, nil
}
