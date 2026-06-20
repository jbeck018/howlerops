package narrative

import (
	"context"
	"fmt"
	"math"
	"strings"
)

// ChatFunc is a provider-agnostic single-shot completion. The host supplies one
// backed by whatever the user configured in AI Settings (Anthropic / OpenAI /
// Ollama / ...), so the narrative service never hard-codes a provider. It mirrors
// the agent package's HostChatFunc.
type ChatFunc func(ctx context.Context, system, prompt string) (string, error)

// Generator produces Insight Briefs from result aggregates.
type Generator struct {
	chat ChatFunc
}

// New returns a Generator backed by the given completion function.
func New(chat ChatFunc) *Generator {
	return &Generator{chat: chat}
}

// ForecastNote is an optional forecast context for the brief. It is a small
// local struct so this package need not depend on internal/forecast; the host
// maps a forecast.Result onto it.
type ForecastNote struct {
	Method      string
	Horizon     int
	First       float64 // first predicted value
	Last        float64 // last predicted value
	LowerLast   float64 // lower CI bound at the last horizon
	UpperLast   float64 // upper CI bound at the last horizon
	MAPEPercent float64 // fit error; NaN if unavailable
}

// AnomalyNote is an optional anomaly callout for the brief.
type AnomalyNote struct {
	When     string
	Observed float64
	Expected float64
}

// BriefInput drives Brief. Summary carries only aggregates (see Summarize).
type BriefInput struct {
	Title     string        // report/component title
	Question  string        // optional user question or focus
	Summary   DataSummary   // aggregate profile of the data
	Forecast  *ForecastNote // optional
	Anomalies []AnomalyNote // optional
}

const systemPrompt = `You are a senior data analyst writing a concise executive insight brief.
You are given ONLY aggregate statistics about a dataset (never raw rows).
Write 3-6 short sentences highlighting the most important trends, magnitudes,
notable categories, and any forecast or anomaly signals. Be specific with the
numbers provided. Do not invent figures you were not given. Do not speculate
about individual records. Plain prose, no preamble.`

// Brief generates an executive narrative. It builds the prompt purely from
// aggregates in BriefInput (no raw rows) and delegates to the configured chat
// function.
func (g *Generator) Brief(ctx context.Context, in BriefInput) (string, error) {
	if g.chat == nil {
		return "", fmt.Errorf("narrative: no chat function configured")
	}
	prompt := buildPrompt(in)
	out, err := g.chat(ctx, systemPrompt, prompt)
	if err != nil {
		return "", fmt.Errorf("narrative: generation failed: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// BuildPrompt exposes the prompt builder for callers that want to inspect or
// cache the exact prompt (and for tests asserting the privacy contract).
func BuildPrompt(in BriefInput) string { return buildPrompt(in) }

func buildPrompt(in BriefInput) string {
	var b strings.Builder

	if t := strings.TrimSpace(in.Title); t != "" {
		fmt.Fprintf(&b, "Report: %s\n", t)
	}
	if q := strings.TrimSpace(in.Question); q != "" {
		fmt.Fprintf(&b, "Focus: %s\n", q)
	}
	fmt.Fprintf(&b, "Rows: %d\n\nColumn aggregates:\n", in.Summary.RowCount)

	for _, c := range in.Summary.Columns {
		fmt.Fprintf(&b, "- %s (%s)", c.Name, c.Kind)
		if c.Nulls > 0 {
			fmt.Fprintf(&b, ", %d nulls", c.Nulls)
		}
		switch c.Kind {
		case KindNumeric:
			fmt.Fprintf(&b, ": min=%s, max=%s, mean=%s, sum=%s",
				num(c.Min), num(c.Max), num(c.Mean), num(c.Sum))
		case KindTemporal:
			fmt.Fprintf(&b, ": from %s to %s", c.Earliest, c.Latest)
		case KindCategorical:
			fmt.Fprintf(&b, ": %d distinct", c.Distinct)
			if len(c.Top) > 0 {
				parts := make([]string, len(c.Top))
				for i, tv := range c.Top {
					parts[i] = fmt.Sprintf("%s (%d)", tv.Value, tv.Count)
				}
				fmt.Fprintf(&b, "; top: %s", strings.Join(parts, ", "))
			}
		}
		b.WriteString("\n")
	}

	if f := in.Forecast; f != nil {
		fmt.Fprintf(&b, "\nForecast (%s, horizon %d): next %s, ending %s [%s, %s]",
			f.Method, f.Horizon, num(f.First), num(f.Last), num(f.LowerLast), num(f.UpperLast))
		if !math.IsNaN(f.MAPEPercent) {
			fmt.Fprintf(&b, "; fit error ~%.1f%%", f.MAPEPercent)
		}
		b.WriteString("\n")
	}

	if len(in.Anomalies) > 0 {
		fmt.Fprintf(&b, "\nAnomalies (%d):\n", len(in.Anomalies))
		limit := len(in.Anomalies)
		if limit > 5 {
			limit = 5
		}
		for _, a := range in.Anomalies[:limit] {
			fmt.Fprintf(&b, "- %s: observed %s vs expected %s\n", a.When, num(a.Observed), num(a.Expected))
		}
	}

	return strings.TrimSpace(b.String())
}

// num formats a float compactly.
func num(f float64) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "n/a"
	}
	if f == math.Trunc(f) {
		return fmt.Sprintf("%.0f", f)
	}
	return fmt.Sprintf("%.2f", f)
}
