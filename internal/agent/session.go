package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
)

// Turn is one completed exchange in a multi-turn session. The json tags keep
// the persisted SessionState shape consistent with its own lowerCamel tags.
type Turn struct {
	UserMessage string `json:"userMessage,omitempty"`
	Answer      string `json:"answer,omitempty"`
	Steps       []Step `json:"steps,omitempty"`
	ExecutedSQL string `json:"executedSql,omitempty"`
}

// SessionOptions configure a multi-turn session.
type SessionOptions struct {
	SystemPrompt string // extra system instructions applied to every turn
	ConnectionID string
	MaxRows      int
	// HistoryLimit caps how many prior turns are fed back as context on each
	// new turn. Defaults to 6.
	HistoryLimit int
}

// Session is a resumable, multi-turn conversation over the agent. Each turn runs
// the ReAct Engine with the accumulated history supplied as context, so the user
// can iterate ("now add a forecast", "break it down by region") like Codex or
// Claude Code. Sessions are serializable via Export/Restore for persistence and
// sync.
type Session struct {
	engine *Engine
	opts   SessionOptions
	turns  []Turn
}

// NewSession starts a fresh multi-turn session.
func (e *Engine) NewSession(opts SessionOptions) *Session {
	if opts.HistoryLimit <= 0 {
		opts.HistoryLimit = 6
	}
	return &Session{engine: e, opts: opts}
}

// Run executes one turn: it assembles the prior history as context, runs the
// agent, and records the exchange.
func (s *Session) Run(ctx context.Context, chatModel model.ToolCallingChatModel, message string) (*Output, error) {
	in := Input{
		Message:      message,
		ConnectionID: s.opts.ConnectionID,
		SystemPrompt: s.opts.SystemPrompt,
		ExtraContext: s.historyContext(),
		MaxRows:      s.opts.MaxRows,
	}
	out, err := s.engine.Run(ctx, chatModel, in)
	if err != nil {
		return nil, err
	}
	s.turns = append(s.turns, Turn{
		UserMessage: message,
		Answer:      out.Answer,
		Steps:       out.Steps,
		ExecutedSQL: out.ExecutedSQL,
	})
	return out, nil
}

// Turns returns a copy of the recorded turns.
func (s *Session) Turns() []Turn {
	out := make([]Turn, len(s.turns))
	copy(out, s.turns)
	return out
}

// historyContext renders the last HistoryLimit turns as plain text for the next
// turn's context.
func (s *Session) historyContext() string {
	if len(s.turns) == 0 {
		return ""
	}
	start := 0
	if len(s.turns) > s.opts.HistoryLimit {
		start = len(s.turns) - s.opts.HistoryLimit
	}
	var b strings.Builder
	b.WriteString("Conversation so far:\n")
	for _, t := range s.turns[start:] {
		fmt.Fprintf(&b, "[USER] %s\n", strings.TrimSpace(t.UserMessage))
		if a := strings.TrimSpace(t.Answer); a != "" {
			fmt.Fprintf(&b, "[ASSISTANT] %s\n", a)
		}
		if sql := strings.TrimSpace(t.ExecutedSQL); sql != "" {
			fmt.Fprintf(&b, "[SQL] %s\n", sql)
		}
	}
	return strings.TrimSpace(b.String())
}

// SessionState is the serializable snapshot of a session, suitable for JSON
// persistence and sync. The host marshals it to durable storage and restores it
// to resume the conversation.
type SessionState struct {
	SystemPrompt string `json:"systemPrompt,omitempty"`
	ConnectionID string `json:"connectionId,omitempty"`
	MaxRows      int    `json:"maxRows,omitempty"`
	HistoryLimit int    `json:"historyLimit,omitempty"`
	Turns        []Turn `json:"turns,omitempty"`
}

// Export captures the session as serializable state.
func (s *Session) Export() SessionState {
	turns := make([]Turn, len(s.turns))
	copy(turns, s.turns)
	return SessionState{
		SystemPrompt: s.opts.SystemPrompt,
		ConnectionID: s.opts.ConnectionID,
		MaxRows:      s.opts.MaxRows,
		HistoryLimit: s.opts.HistoryLimit,
		Turns:        turns,
	}
}

// RestoreSession rebuilds a session from persisted state so a conversation can
// resume where it left off.
func (e *Engine) RestoreSession(state SessionState) *Session {
	opts := SessionOptions{
		SystemPrompt: state.SystemPrompt,
		ConnectionID: state.ConnectionID,
		MaxRows:      state.MaxRows,
		HistoryLimit: state.HistoryLimit,
	}
	if opts.HistoryLimit <= 0 {
		opts.HistoryLimit = 6
	}
	s := &Session{engine: e, opts: opts}
	s.turns = make([]Turn, len(state.Turns))
	copy(s.turns, state.Turns)
	return s
}
