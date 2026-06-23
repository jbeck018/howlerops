package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
)

// Specialist is a named sub-agent persona: a description used for routing and a
// system prompt that focuses the agent for a class of work (e.g. "sql_analyst",
// "report_builder", "narrator"). Specialists share the Engine's toolset today;
// per-specialist tool subsets are a future extension.
type Specialist struct {
	Name         string
	Description  string
	SystemPrompt string
}

// RouterFunc selects which specialist should handle a message. It returns the
// chosen specialist's name. A nil router falls back to KeywordRouter. The LLM
// can drive routing by supplying a RouterFunc that asks the model to choose.
type RouterFunc func(ctx context.Context, message string, specialists []Specialist) (string, error)

// Orchestrator coordinates multiple specialists over a single multi-turn
// session: each turn is routed to a specialist whose system prompt is applied
// for that turn, while the conversation history is shared across specialists.
// This is the multi-agent, multi-turn substrate for NL reporting.
type Orchestrator struct {
	engine      *Engine
	specialists []Specialist
	byName      map[string]Specialist
	router      RouterFunc
	session     *Session
	baseSystem  string
}

// OrchestratorOutput is a turn result annotated with the specialist that handled
// it.
type OrchestratorOutput struct {
	Specialist string
	*Output
}

// NewOrchestrator builds an orchestrator over the given specialists. opts.SystemPrompt
// is treated as a base prompt prepended to the chosen specialist's prompt each
// turn. A nil router uses KeywordRouter.
func (e *Engine) NewOrchestrator(specialists []Specialist, router RouterFunc, opts SessionOptions) (*Orchestrator, error) {
	if len(specialists) == 0 {
		return nil, fmt.Errorf("agent: orchestrator needs at least one specialist")
	}
	byName := make(map[string]Specialist, len(specialists))
	for _, s := range specialists {
		if s.Name == "" {
			return nil, fmt.Errorf("agent: specialist with empty name")
		}
		if _, dup := byName[s.Name]; dup {
			return nil, fmt.Errorf("agent: duplicate specialist %q", s.Name)
		}
		byName[s.Name] = s
	}
	if router == nil {
		router = KeywordRouter
	}
	base := opts.SystemPrompt
	// The session's per-turn system prompt is set dynamically per specialist, so
	// clear it on the shared session and keep the base here.
	sessOpts := opts
	sessOpts.SystemPrompt = ""
	return &Orchestrator{
		engine:      e,
		specialists: specialists,
		byName:      byName,
		router:      router,
		session:     e.NewSession(sessOpts),
		baseSystem:  base,
	}, nil
}

// Run routes the message to a specialist and executes one turn against the
// shared session.
func (o *Orchestrator) Run(ctx context.Context, chatModel model.ToolCallingChatModel, message string) (*OrchestratorOutput, error) {
	name, err := o.router(ctx, message, o.specialists)
	if err != nil {
		return nil, fmt.Errorf("agent: routing failed: %w", err)
	}
	spec, ok := o.byName[name]
	if !ok {
		return nil, fmt.Errorf("agent: router chose unknown specialist %q", name)
	}

	// Apply the specialist's system prompt for this turn.
	o.session.opts.SystemPrompt = composeSystemPrompt(o.baseSystem, spec.SystemPrompt)
	out, err := o.session.Run(ctx, chatModel, message)
	if err != nil {
		return nil, err
	}
	return &OrchestratorOutput{Specialist: spec.Name, Output: out}, nil
}

// Session exposes the shared multi-turn session for export/inspection.
func (o *Orchestrator) Session() *Session { return o.session }

func composeSystemPrompt(base, specialist string) string {
	base, specialist = strings.TrimSpace(base), strings.TrimSpace(specialist)
	switch {
	case base == "":
		return specialist
	case specialist == "":
		return base
	default:
		return base + "\n\n" + specialist
	}
}

// KeywordRouter is the default router: it tokenizes the message and scores each
// specialist by how many message words match the specialist's name/description
// keywords (exact, or a shared prefix of length >= 4 so "forecast" matches
// "forecaster"). It picks the highest-scoring specialist and falls back to the
// first. Deterministic and dependency-free; swap in an LLM-backed RouterFunc for
// smarter routing.
func KeywordRouter(_ context.Context, message string, specialists []Specialist) (string, error) {
	if len(specialists) == 0 {
		return "", fmt.Errorf("agent: no specialists to route to")
	}
	words := tokenize(message)
	best := specialists[0].Name
	bestScore := 0
	for _, s := range specialists {
		kws := keywordSet(s)
		score := 0
		for w := range words {
			for kw := range kws {
				if matchToken(w, kw) {
					score++
					break
				}
			}
		}
		if score > bestScore {
			bestScore = score
			best = s.Name
		}
	}
	return best, nil
}

// matchToken reports whether two lowercased tokens refer to the same concept:
// equal, or sharing a prefix of at least 4 characters (to absorb plural/verb
// variants like forecast/forecaster without matching on tiny fragments).
func matchToken(a, b string) bool {
	if a == b {
		return true
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if n < 4 {
		return false
	}
	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

// tokenize splits text into a set of lowercased alphanumeric words.
func tokenize(text string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}) {
		if len(w) >= 3 {
			out[w] = true
		}
	}
	return out
}

// keywordSet builds the keyword set for a specialist from its name (and name
// parts) and description words.
func keywordSet(s Specialist) map[string]bool {
	kws := tokenize(s.Name + " " + s.Description)
	kws[strings.ToLower(s.Name)] = true
	return kws
}
