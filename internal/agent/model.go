package agent

import (
	"context"
	"strings"
	"time"

	einoclaude "github.com/cloudwego/eino-ext/components/model/claude"
	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// HostChatFunc lets the host drive providers that lack native tool calling
// (for example the Claude CLI, which authenticates as a local subprocess). It
// receives the flattened system prompt and conversation text and returns the
// assistant's reply.
type HostChatFunc func(ctx context.Context, system, prompt string) (string, error)

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

	// ChatFunc, when set, is used instead of an HTTP model. It powers providers
	// without native tool calling (e.g. the Claude CLI), running as a graceful
	// single-shot path inside the ReAct loop.
	ChatFunc HostChatFunc
}

// BuildModel maps the app's provider configuration onto an Eino tool-calling
// chat model. OpenAI, Codex, Ollama, and HuggingFace (plus any OpenAI-compatible
// local CLI or proxy) are served through the OpenAI-compatible client with a
// custom base URL — which is exactly how local-CLI auth keeps working. Anthropic
// is served through the Claude client. Providers that supply a ChatFunc (the
// Claude CLI) are served through a non-tool-calling function model.
func BuildModel(ctx context.Context, cfg ModelConfig) (model.ToolCallingChatModel, error) {
	if cfg.ChatFunc != nil {
		return newFuncModel(cfg.ChatFunc), nil
	}

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

// funcModel adapts a HostChatFunc to Eino's ToolCallingChatModel. It does not
// emit tool calls, so the ReAct loop treats its reply as the final answer. Tools
// are accepted (and ignored) so the model satisfies the interface and can still
// receive tool schemas in its prompt context if desired.
type funcModel struct {
	fn HostChatFunc
}

func newFuncModel(fn HostChatFunc) model.ToolCallingChatModel {
	return &funcModel{fn: fn}
}

func (m *funcModel) Generate(ctx context.Context, in []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	system, prompt := flattenMessages(in)
	out, err := m.fn(ctx, system, prompt)
	if err != nil {
		return nil, err
	}
	return schema.AssistantMessage(out, nil), nil
}

func (m *funcModel) Stream(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := m.Generate(ctx, in, opts...)
	if err != nil {
		return nil, err
	}
	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		sw.Send(msg, nil)
		sw.Close()
	}()
	return sr, nil
}

func (m *funcModel) WithTools(_ []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

func flattenMessages(in []*schema.Message) (system, prompt string) {
	var sys strings.Builder
	var body strings.Builder
	for _, msg := range in {
		if msg == nil {
			continue
		}
		if msg.Role == schema.System {
			sys.WriteString(msg.Content)
			sys.WriteString("\n")
			continue
		}
		role := string(msg.Role)
		if role == "" {
			role = "user"
		}
		body.WriteString(strings.ToUpper(role))
		body.WriteString(": ")
		body.WriteString(msg.Content)
		body.WriteString("\n")
	}
	return strings.TrimSpace(sys.String()), strings.TrimSpace(body.String())
}
