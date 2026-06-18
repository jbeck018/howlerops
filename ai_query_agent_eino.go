package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jbeck018/howlerops/internal/agent"
)

// wailsAgentTools adapts the existing WailsAIService database, schema, and
// memory capabilities to the agent.Toolset interface consumed by the Eino agent.
type wailsAgentTools struct {
	s *WailsAIService
}

func (t *wailsAgentTools) Schema(_ context.Context, connectionID string) (string, error) {
	if strings.TrimSpace(connectionID) == "" {
		return "", nil
	}
	return t.s.buildDetailedSchemaContext(connectionID), nil
}

func (t *wailsAgentTools) RunSQL(_ context.Context, connectionID, sql string) (*agent.SQLResult, error) {
	if strings.TrimSpace(connectionID) == "" {
		return nil, fmt.Errorf("no active database connection")
	}
	res, err := t.s.ExecuteReadOnlyQueryWithPagination(connectionID, sql, 200, 0, 30*time.Second)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return &agent.SQLResult{}, nil
	}
	return &agent.SQLResult{
		Columns:         res.Columns,
		Rows:            res.Rows,
		RowCount:        res.RowCount,
		ExecutionTimeMs: res.ExecutionTimeMs,
		Limited:         res.Limited,
	}, nil
}

func (t *wailsAgentTools) SearchMemory(_ context.Context, query string, limit int) (string, error) {
	results, err := t.s.RecallAIMemorySessions(query, limit)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "", nil
	}
	b, err := json.Marshal(results)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// buildAgentModelConfig assembles the credentials for the requested provider
// from the service's configured AI settings, preserving existing auth (API
// keys / base URLs for Codex and other OpenAI-compatible local CLIs).
func (s *WailsAIService) buildAgentModelConfig(provider, model string) agent.ModelConfig {
	cfg := agent.ModelConfig{Provider: provider, Model: model}
	if s.aiConfig == nil {
		return cfg
	}

	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai":
		cfg.APIKey = s.aiConfig.OpenAI.APIKey
		cfg.BaseURL = s.aiConfig.OpenAI.BaseURL
		if model == "" && len(s.aiConfig.OpenAI.Models) > 0 {
			cfg.Model = s.aiConfig.OpenAI.Models[0]
		}
	case "codex":
		cfg.APIKey = s.aiConfig.Codex.APIKey
		cfg.BaseURL = s.aiConfig.Codex.BaseURL
		cfg.Organization = s.aiConfig.Codex.Organization
		if model == "" {
			cfg.Model = s.aiConfig.Codex.Model
		}
	case "anthropic", "claudecode":
		cfg.APIKey = s.aiConfig.Anthropic.APIKey
		cfg.BaseURL = s.aiConfig.Anthropic.BaseURL
		if model == "" && len(s.aiConfig.Anthropic.Models) > 0 {
			cfg.Model = s.aiConfig.Anthropic.Models[0]
		}
	case "ollama":
		cfg.BaseURL = openAICompatBaseURL(s.aiConfig.Ollama.Endpoint)
		cfg.APIKey = "ollama"
		if model == "" && len(s.aiConfig.Ollama.Models) > 0 {
			cfg.Model = s.aiConfig.Ollama.Models[0]
		}
	case "huggingface":
		cfg.BaseURL = openAICompatBaseURL(s.aiConfig.HuggingFace.Endpoint)
		cfg.APIKey = "huggingface"
		if model == "" {
			cfg.Model = s.aiConfig.HuggingFace.RecommendedModel
		}
	}
	return cfg
}

// openAICompatBaseURL normalises a local endpoint to its OpenAI-compatible path.
func openAICompatBaseURL(endpoint string) string {
	ep := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if ep == "" {
		return ""
	}
	if !strings.HasSuffix(ep, "/v1") {
		ep += "/v1"
	}
	return ep
}

func readOnlyResultFromAgent(r *agent.SQLResult, connectionID string) *ReadOnlyQueryResult {
	if r == nil {
		return nil
	}
	return &ReadOnlyQueryResult{
		Columns:         r.Columns,
		Rows:            r.Rows,
		RowCount:        r.RowCount,
		ExecutionTimeMs: r.ExecutionTimeMs,
		Limited:         r.Limited,
		ConnectionID:    connectionID,
	}
}

func toolStepTitle(toolName string) string {
	switch toolName {
	case "get_schema":
		return "Schema Lookup"
	case "search_memory":
		return "Memory Recall"
	default:
		return toolName
	}
}

func toolStepSummary(step agent.Step) string {
	switch step.Tool {
	case "get_schema":
		return "Inspected the database schema."
	case "search_memory":
		if strings.TrimSpace(step.Input) != "" {
			return fmt.Sprintf("Recalled context for: %s", step.Input)
		}
		return "Searched memory for relevant context."
	default:
		if strings.TrimSpace(step.Output) != "" {
			return step.Output
		}
		return step.Tool
	}
}
