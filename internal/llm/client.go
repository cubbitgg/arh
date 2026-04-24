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
	default:
		return nil, fmt.Errorf("LLM provider %q is not implemented in this version; supported: anthropic", cfg.Provider)
	}
}
