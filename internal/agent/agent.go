// Package agent provides the live, Eino-powered tool-calling agent that backs
// HowlerOps' in-app AI assistant. It replaces the previous hand-rolled,
// single-shot prompt chain with a real ReAct loop: the model decides when to
// fetch schema, recall memory, and run read-only SQL, then synthesises a final
// answer.
package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
)

// Toolset is the set of capabilities the agent can call. The host application
// implements this over its existing database, schema, and memory services so
// the agent stays decoupled from Wails and the rest of the app.
type Toolset interface {
	// Schema returns a textual description of the connection's database schema.
	Schema(ctx context.Context, connectionID string) (string, error)
	// RunSQL executes a read-only SQL statement and returns the result.
	RunSQL(ctx context.Context, connectionID, sql string) (*SQLResult, error)
	// SearchMemory returns relevant remembered context for the query.
	SearchMemory(ctx context.Context, query string, limit int) (string, error)
}

// SQLResult is the structured result of a read-only query.
type SQLResult struct {
	Columns         []string
	Rows            []map[string]interface{}
	RowCount        int64
	ExecutionTimeMs int64
	Limited         bool
	Error           string
}

// Input is a single agent turn.
type Input struct {
	Message      string
	ConnectionID string
	SystemPrompt string // optional extra system instructions from the host
	ExtraContext string // schema context / history already assembled by the host
	MaxRows      int
}

// Step records a single tool invocation the agent made, for surfacing in the UI.
type Step struct {
	Tool   string
	Input  string
	Output string
	SQL    string
	Result *SQLResult
}

// Output is the result of running the agent.
type Output struct {
	Answer      string
	Steps       []Step
	ExecutedSQL string
	LastResult  *SQLResult
}

// Engine wraps an Eino ReAct agent configured with database tools.
type Engine struct {
	tools          Toolset
	defaultMaxRows int
	maxSteps       int
}

// New creates an Engine backed by the given Toolset.
func New(tools Toolset) *Engine {
	return &Engine{tools: tools, defaultMaxRows: 200, maxSteps: 20}
}

const baseSystemPrompt = `You are HowlerOps' database analyst agent. You help users explore and query their SQL databases.

You can call these tools:
- get_schema: fetch the tables and columns for the active connection. Call this before writing SQL unless the schema is already provided in context.
- run_sql: execute a single READ-ONLY SQL SELECT (or CTE) statement and get rows back. Never attempt INSERT/UPDATE/DELETE/DDL.
- search_memory: recall relevant context, prior queries, and saved knowledge from memory.
- forecast: project a numeric time series into the future (with confidence intervals) and flag historical anomalies. Pass a read-only SQL query that returns a timestamp column and a numeric value column. Use it for trend/projection questions ("what will revenue be next month") and outlier detection.

Guidelines:
- To answer data questions, prefer running a query over guessing. Never fabricate rows or results.
- Write standard SQL for the user's database dialect. Always use LIMIT to keep result sets small.
- After running SQL, summarise the findings in clear, concise prose.
- If a tool returns an error, explain it plainly and suggest a fix.`

// Run executes one agent turn using the supplied tool-calling chat model.
func (e *Engine) Run(ctx context.Context, chatModel model.ToolCallingChatModel, in Input) (*Output, error) {
	if chatModel == nil {
		return nil, fmt.Errorf("agent: chat model is nil")
	}

	maxRows := in.MaxRows
	if maxRows <= 0 {
		maxRows = e.defaultMaxRows
	}

	rs := &runState{}
	tools, err := e.buildTools(rs, in.ConnectionID, maxRows)
	if err != nil {
		return nil, fmt.Errorf("agent: build tools: %w", err)
	}

	sysPrompt := buildSystemPrompt(in)
	reactAgent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: chatModel,
		ToolsConfig:      compose.ToolsNodeConfig{Tools: tools},
		MessageModifier: func(_ context.Context, input []*schema.Message) []*schema.Message {
			return append([]*schema.Message{schema.SystemMessage(sysPrompt)}, input...)
		},
		MaxStep: e.maxSteps,
	})
	if err != nil {
		return nil, fmt.Errorf("agent: init react agent: %w", err)
	}

	final, err := reactAgent.Generate(ctx, []*schema.Message{schema.UserMessage(in.Message)})
	if err != nil {
		return nil, fmt.Errorf("agent: generate: %w", err)
	}

	answer := ""
	if final != nil {
		answer = strings.TrimSpace(final.Content)
	}

	return &Output{
		Answer:      answer,
		Steps:       rs.snapshot(),
		ExecutedSQL: rs.executedSQL,
		LastResult:  rs.lastResult,
	}, nil
}

// runState collects tool activity during a single Run.
type runState struct {
	mu          sync.Mutex
	steps       []Step
	executedSQL string
	lastResult  *SQLResult
}

func (r *runState) record(s Step) {
	r.mu.Lock()
	r.steps = append(r.steps, s)
	r.mu.Unlock()
}

func (r *runState) snapshot() []Step {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Step, len(r.steps))
	copy(out, r.steps)
	return out
}

type getSchemaInput struct {
	Reason string `json:"reason,omitempty" jsonschema_description:"Why you need the schema (optional)"`
}

type runSQLInput struct {
	SQL string `json:"sql" jsonschema:"required" jsonschema_description:"A single read-only SQL SELECT or CTE statement to execute"`
}

type searchMemoryInput struct {
	Query string `json:"query" jsonschema:"required" jsonschema_description:"What to recall from memory"`
	Limit int    `json:"limit,omitempty" jsonschema_description:"Maximum number of memory snippets to return (default 5)"`
}

func (e *Engine) buildTools(rs *runState, connectionID string, maxRows int) ([]tool.BaseTool, error) {
	getSchema, err := utils.InferTool(
		"get_schema",
		"Fetch the database schema (tables and columns) for the active connection.",
		func(ctx context.Context, _ getSchemaInput) (string, error) {
			s, err := e.tools.Schema(ctx, connectionID)
			rs.record(Step{Tool: "get_schema", Output: truncate(s, 4000)})
			if err != nil {
				return fmt.Sprintf("Failed to load schema: %v", err), nil
			}
			if strings.TrimSpace(s) == "" {
				return "No schema is available for this connection.", nil
			}
			return truncate(s, 12000), nil
		},
	)
	if err != nil {
		return nil, err
	}

	runSQL, err := utils.InferTool(
		"run_sql",
		"Execute a read-only SQL SELECT/CTE statement against the active connection and return the rows.",
		func(ctx context.Context, args runSQLInput) (string, error) {
			sql := strings.TrimSpace(args.SQL)
			if sql == "" {
				return "Error: empty SQL statement.", nil
			}
			step := Step{Tool: "run_sql", SQL: sql}
			res, err := e.tools.RunSQL(ctx, connectionID, sql)
			if err != nil {
				step.Output = "error: " + err.Error()
				rs.record(step)
				return fmt.Sprintf("Query failed: %v", err), nil
			}
			rs.mu.Lock()
			rs.executedSQL = sql
			rs.lastResult = res
			rs.mu.Unlock()
			formatted := formatResult(res, maxRows)
			step.Result = res
			step.Output = formatted
			rs.record(step)
			return formatted, nil
		},
	)
	if err != nil {
		return nil, err
	}

	searchMemory, err := utils.InferTool(
		"search_memory",
		"Recall relevant context, prior queries, and saved knowledge from memory.",
		func(ctx context.Context, args searchMemoryInput) (string, error) {
			limit := args.Limit
			if limit <= 0 {
				limit = 5
			}
			out, err := e.tools.SearchMemory(ctx, args.Query, limit)
			rs.record(Step{Tool: "search_memory", Input: args.Query, Output: truncate(out, 2000)})
			if err != nil {
				return fmt.Sprintf("Memory search failed: %v", err), nil
			}
			if strings.TrimSpace(out) == "" {
				return "No relevant memory found.", nil
			}
			return truncate(out, 6000), nil
		},
	)
	if err != nil {
		return nil, err
	}

	forecastTool, err := e.buildForecastTool(rs, connectionID)
	if err != nil {
		return nil, err
	}

	return []tool.BaseTool{getSchema, runSQL, searchMemory, forecastTool}, nil
}

func buildSystemPrompt(in Input) string {
	var b strings.Builder
	b.WriteString(baseSystemPrompt)
	if s := strings.TrimSpace(in.SystemPrompt); s != "" {
		b.WriteString("\n\nAdditional instructions:\n")
		b.WriteString(s)
	}
	if c := strings.TrimSpace(in.ExtraContext); c != "" {
		b.WriteString("\n\nKnown context (schema / history / memory):\n")
		b.WriteString(truncate(c, 8000))
	}
	if strings.TrimSpace(in.ConnectionID) == "" {
		b.WriteString("\n\nNote: there is no active database connection, so run_sql is unavailable. Answer from knowledge and memory only.")
	}
	return b.String()
}

func formatResult(r *SQLResult, maxRows int) string {
	if r == nil {
		return "No result."
	}
	if r.Error != "" {
		return "Error: " + r.Error
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Returned %d row(s).\n", r.RowCount)
	if len(r.Columns) > 0 {
		b.WriteString("Columns: " + strings.Join(r.Columns, ", ") + "\n")
	}
	limit := maxRows
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	for i, row := range r.Rows {
		if i >= limit {
			fmt.Fprintf(&b, "... (%d more row(s))\n", len(r.Rows)-limit)
			break
		}
		cells := make([]string, 0, len(r.Columns))
		for _, c := range r.Columns {
			cells = append(cells, fmt.Sprintf("%v", row[c]))
		}
		b.WriteString(strings.Join(cells, " | ") + "\n")
	}
	return truncate(strings.TrimSpace(b.String()), 6000)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…(truncated)"
}
