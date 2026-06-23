package agent

import (
	"context"
	"errors"
	"testing"
)

func testSpecialists() []Specialist {
	return []Specialist{
		{Name: "sql_analyst", Description: "writes and runs queries", SystemPrompt: "You write SQL."},
		{Name: "forecaster", Description: "projects trends and forecasts", SystemPrompt: "You forecast."},
		{Name: "narrator", Description: "writes executive narrative summaries", SystemPrompt: "You narrate."},
	}
}

func TestKeywordRouter_PicksByKeyword(t *testing.T) {
	specs := testSpecialists()
	cases := map[string]string{
		"please forecast next quarter revenue": "forecaster",
		"write a narrative summary":            "narrator",
		"run a sql_analyst query":              "sql_analyst",
	}
	for msg, want := range cases {
		got, err := KeywordRouter(context.Background(), msg, specs)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("router(%q) = %q, want %q", msg, got, want)
		}
	}
}

func TestKeywordRouter_FallsBackToFirst(t *testing.T) {
	specs := testSpecialists()
	got, err := KeywordRouter(context.Background(), "something unrelated entirely", specs)
	if err != nil {
		t.Fatal(err)
	}
	if got != "sql_analyst" {
		t.Errorf("expected fallback to first specialist, got %q", got)
	}
}

func TestKeywordRouter_EmptySpecialists(t *testing.T) {
	if _, err := KeywordRouter(context.Background(), "anything", nil); err == nil {
		t.Error("expected error (not a panic) for empty specialists")
	}
}

func TestOrchestrator_RoutesAndAppliesPrompt(t *testing.T) {
	e := New(fakeTools{})
	// Deterministic router that always picks the forecaster.
	router := func(_ context.Context, _ string, _ []Specialist) (string, error) {
		return "forecaster", nil
	}
	o, err := e.NewOrchestrator(testSpecialists(), router, SessionOptions{SystemPrompt: "base"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := o.Run(context.Background(), &fakeModel{reply: "projected"}, "what's next")
	if err != nil {
		t.Fatal(err)
	}
	if out.Specialist != "forecaster" {
		t.Errorf("specialist = %q, want forecaster", out.Specialist)
	}
	if out.Answer != "projected" {
		t.Errorf("answer = %q", out.Answer)
	}
	// The chosen specialist's prompt (composed with the base) is applied to the
	// shared session.
	if o.session.opts.SystemPrompt != "base\n\nYou forecast." {
		t.Errorf("composed system prompt = %q", o.session.opts.SystemPrompt)
	}
}

func TestOrchestrator_SharedHistoryAcrossSpecialists(t *testing.T) {
	e := New(fakeTools{})
	// Router picks specialist by first word of the message.
	router := func(_ context.Context, message string, _ []Specialist) (string, error) {
		if len(message) > 0 && message[0] == 'f' {
			return "forecaster", nil
		}
		return "sql_analyst", nil
	}
	o, err := e.NewOrchestrator(testSpecialists(), router, SessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := o.Run(context.Background(), &fakeModel{reply: "a1"}, "select something"); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Run(context.Background(), &fakeModel{reply: "a2"}, "forecast it"); err != nil {
		t.Fatal(err)
	}
	// Both turns are recorded in one shared session regardless of specialist.
	if len(o.Session().Turns()) != 2 {
		t.Errorf("expected 2 shared turns, got %d", len(o.Session().Turns()))
	}
}

func TestOrchestrator_RouterError(t *testing.T) {
	e := New(fakeTools{})
	router := func(_ context.Context, _ string, _ []Specialist) (string, error) {
		return "", errors.New("router boom")
	}
	o, err := e.NewOrchestrator(testSpecialists(), router, SessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := o.Run(context.Background(), &fakeModel{reply: "x"}, "msg"); err == nil {
		t.Error("expected error when router fails")
	}
}

func TestOrchestrator_UnknownSpecialistChosen(t *testing.T) {
	e := New(fakeTools{})
	router := func(_ context.Context, _ string, _ []Specialist) (string, error) {
		return "ghost", nil
	}
	o, err := e.NewOrchestrator(testSpecialists(), router, SessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := o.Run(context.Background(), &fakeModel{reply: "x"}, "msg"); err == nil {
		t.Error("expected error when router returns an unknown specialist")
	}
}

func TestNewOrchestrator_Validation(t *testing.T) {
	e := New(fakeTools{})
	if _, err := e.NewOrchestrator(nil, nil, SessionOptions{}); err == nil {
		t.Error("expected error for no specialists")
	}
	dup := []Specialist{{Name: "a"}, {Name: "a"}}
	if _, err := e.NewOrchestrator(dup, nil, SessionOptions{}); err == nil {
		t.Error("expected error for duplicate specialist names")
	}
	empty := []Specialist{{Name: ""}}
	if _, err := e.NewOrchestrator(empty, nil, SessionOptions{}); err == nil {
		t.Error("expected error for empty specialist name")
	}
}
