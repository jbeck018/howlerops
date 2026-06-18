package agent

import (
	"context"
	"strings"
	"time"

	einoclaude "github.com/cloudwego/eino-ext/components/model/claude"
	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

// ModelConfig carries the provider credentials needed to build an Eino chat
// model. It mirrors the app's existing provider configuration so authentication
// (API keys, base URLs for Codex and other local/OpenAI-compatible CLIs) is
// preserved exactly.
type ModelConfig struct {
	Provider     string // openai, anthropic, ollama, huggingface, codex, claudecode
	Model        string
	APIKey       string
	BaseURL      string
	Organization string
	MaxTokens    int
	Temperature  float64
	Timeout      time.Duration
}

// BuildModel maps the app's provider configuration onto an Eino tool-calling
// chat model. OpenAI, Codex, Ollama, and HuggingFace (plus any OpenAI-compatible
// local CLI or proxy) are served through the OpenAI-compatible client with a
// custom base URL — which is exactly how local-CLI auth keeps working. Anthropic
// and ClaudeCode are served through the Claude client.
func BuildModel(ctx context.Context, cfg ModelConfig) (model.ToolCallingChatModel, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	modelName := defaultModel(provider, cfg.Model)

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}

	switch provider {
	case "anthropic", "claude", "claudecode":
		c := &einoclaude.Config{
			APIKey:    cfg.APIKey,
			Model:     modelName,
			MaxTokens: cfg.MaxTokens,
		}
		if c.MaxTokens <= 0 {
			c.MaxTokens = 4096
		}
		if base := strings.TrimSpace(cfg.BaseURL); base != "" {
			c.BaseURL = &base
		}
		if cfg.Temperature > 0 {
			t := float32(cfg.Temperature)
			c.Temperature = &t
		}
		return einoclaude.NewChatModel(ctx, c)

	default:
		// openai, codex, ollama, huggingface, and any OpenAI-compatible endpoint.
		c := &einoopenai.ChatModelConfig{
			APIKey:  cfg.APIKey,
			Model:   modelName,
			BaseURL: strings.TrimSpace(cfg.BaseURL),
			Timeout: timeout,
		}
		if cfg.MaxTokens > 0 {
			mt := cfg.MaxTokens
			c.MaxTokens = &mt
		}
		if cfg.Temperature > 0 {
			t := float32(cfg.Temperature)
			c.Temperature = &t
		}
		return einoopenai.NewChatModel(ctx, c)
	}
}

func defaultModel(provider, model string) string {
	if m := strings.TrimSpace(model); m != "" {
		return m
	}
	switch strings.ToLower(provider) {
	case "anthropic", "claude", "claudecode":
		return "claude-3-5-sonnet-20241022"
	case "ollama":
		return "llama3.1"
	case "huggingface":
		return "meta-llama/Meta-Llama-3-8B-Instruct"
	default:
		return "gpt-4o-mini"
	}
}
