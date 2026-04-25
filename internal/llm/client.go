package llm

import (
	"context"
	"fmt"
	"os"

	"github.com/cubbitgg/arh/internal/config"
)

// LLMClient sends a prompt to an LLM and returns the response.
type LLMClient interface {
	Complete(ctx context.Context, systemPrompt string, userPrompt string) (string, error)
}

// NewClient creates an LLMClient from the provider config.
func NewClient(cfg config.LLMProviderConfig) (LLMClient, error) {
	apiKey := ""
	if cfg.APIKeyEnv != "" {
		apiKey = os.Getenv(cfg.APIKeyEnv)
	}

	switch cfg.Provider {
	case "anthropic", "":
		if apiKey == "" {
			return nil, fmt.Errorf("environment variable %s is not set", cfg.APIKeyEnv)
		}
		return NewAnthropicClient(apiKey, cfg.Model), nil
	case "openai":
		if apiKey == "" {
			return nil, fmt.Errorf("environment variable %s is not set (required for openai provider)", cfg.APIKeyEnv)
		}
		return NewOpenAIClient(apiKey, cfg.Model), nil
	case "ollama":
		if cfg.Model == "" {
			return nil, fmt.Errorf("ollama provider requires a model name in config (e.g. model: qwen2.5-coder:14b)")
		}
		return NewOllamaClient(cfg.Endpoint, cfg.Model), nil
	default:
		return nil, fmt.Errorf("LLM provider %q is not supported; valid options: anthropic, openai, ollama", cfg.Provider)
	}
}
