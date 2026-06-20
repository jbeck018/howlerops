package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jbeck018/howlerops/internal/insight"
	"github.com/jbeck018/howlerops/pkg/ai"
)

// maxInsightRows bounds how many rows are pulled for a brief/forecast so a large
// table can't blow up memory or the prompt. Aggregation/forecasting work fine on
// a generous prefix; tighten via the component's own LIMIT for precise control.
const maxInsightRows = 5000

// InsightBriefRequest drives GenerateInsightBrief.
type InsightBriefRequest struct {
	ConnectionID string `json:"connectionId"`
	SQL          string `json:"sql"`
	Title        string `json:"title,omitempty"`
	Question     string `json:"question,omitempty"`
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`

	// Forecast requests a projection appended to the brief. Columns are
	// auto-detected when omitted.
	Forecast     bool   `json:"forecast,omitempty"`
	TimeColumn   string `json:"timeColumn,omitempty"`
	ValueColumn  string `json:"valueColumn,omitempty"`
	Horizon      int    `json:"horizon,omitempty"`
	SeasonLength int    `json:"seasonLength,omitempty"`
}

// InsightForecastPoint is one predicted period with its confidence interval.
type InsightForecastPoint struct {
	Time  string  `json:"time"`
	Value float64 `json:"value"`
	Lower float64 `json:"lower"`
	Upper float64 `json:"upper"`
}

// InsightAnomaly is a flagged historical observation.
type InsightAnomaly struct {
	Time     string  `json:"time"`
	Value    float64 `json:"value"`
	Expected float64 `json:"expected"`
	Score    float64 `json:"score"`
}

// InsightBriefResponse is the wire shape returned to the frontend.
type InsightBriefResponse struct {
	Brief          string                 `json:"brief"`
	RowCount       int                    `json:"rowCount"`
	ForecastMethod string                 `json:"forecastMethod,omitempty"`
	Predictions    []InsightForecastPoint `json:"predictions,omitempty"`
	Anomalies      []InsightAnomaly       `json:"anomalies,omitempty"`
	ForecastError  string                 `json:"forecastError,omitempty"`
}

// GenerateInsightBrief runs a read-only query and produces an Auto Insight Brief
// (executive narrative + optional forecast/anomalies) using the user's
// configured AI provider. Only aggregates are sent to the model — never raw rows
// (see internal/narrative).
func (s *WailsAIService) GenerateInsightBrief(req InsightBriefRequest) (*InsightBriefResponse, error) {
	if strings.TrimSpace(req.ConnectionID) == "" {
		return nil, fmt.Errorf("no active database connection")
	}
	if strings.TrimSpace(req.SQL) == "" {
		return nil, fmt.Errorf("query is empty")
	}
	if s.aiService == nil {
		return nil, fmt.Errorf("AI service not configured")
	}

	result, err := s.ExecuteReadOnlyQueryWithPagination(req.ConnectionID, req.SQL, maxInsightRows, 0, 60*time.Second)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("query returned no result")
	}

	ctx := context.Background()
	out, err := insight.Generate(ctx, s.insightChatFunc(req.Provider, req.Model), result.Columns, result.Rows, insight.Options{
		Title:       req.Title,
		Question:    req.Question,
		Forecast:    req.Forecast,
		TimeColumn:  req.TimeColumn,
		ValueColumn: req.ValueColumn,
		Horizon:     req.Horizon,
		Season:      req.SeasonLength,
	})
	if err != nil {
		return nil, err
	}

	resp := &InsightBriefResponse{
		Brief:         out.Brief,
		RowCount:      out.Summary.RowCount,
		ForecastError: out.ForecastErr,
	}
	if out.Forecast != nil {
		resp.ForecastMethod = string(out.Forecast.Method)
		for _, p := range out.Forecast.Predictions {
			resp.Predictions = append(resp.Predictions, InsightForecastPoint{
				Time:  p.Time.UTC().Format(time.RFC3339),
				Value: p.Value,
				Lower: p.Lower,
				Upper: p.Upper,
			})
		}
	}
	for _, a := range out.Anomalies {
		resp.Anomalies = append(resp.Anomalies, InsightAnomaly{
			Time:     a.Time.UTC().Format(time.RFC3339),
			Value:    a.Value,
			Expected: a.Expected,
			Score:    a.Score,
		})
	}
	return resp, nil
}

// insightChatFunc returns a single-shot completion bound to the user's
// configured provider, used by the narrative service. An empty provider falls
// back to the AI service's configured default.
func (s *WailsAIService) insightChatFunc(provider, model string) func(context.Context, string, string) (string, error) {
	return func(ctx context.Context, system, prompt string) (string, error) {
		if s.aiService == nil {
			return "", fmt.Errorf("AI service not configured")
		}
		resp, err := s.aiService.Chat(ctx, &ai.ChatRequest{
			System:   system,
			Prompt:   prompt,
			Provider: provider,
			Model:    model,
		})
		if err != nil {
			return "", err
		}
		if resp == nil {
			return "", fmt.Errorf("AI provider returned no response")
		}
		return resp.Content, nil
	}
}
