package agent

import (
	"context"
	"strings"
	"testing"
)

func TestSession_MultiTurnRecordsHistory(t *testing.T) {
	e := New(fakeTools{})
	sess := e.NewSession(SessionOptions{ConnectionID: "c1"})

	if _, err := sess.Run(context.Background(), &fakeModel{reply: "first answer"}, "first question"); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Run(context.Background(), &fakeModel{reply: "second answer"}, "second question"); err != nil {
		t.Fatal(err)
	}

	turns := sess.Turns()
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(turns))
	}
	if turns[0].UserMessage != "first question" || turns[0].Answer != "first answer" {
		t.Errorf("turn 0 wrong: %+v", turns[0])
	}
	if turns[1].Answer != "second answer" {
		t.Errorf("turn 1 wrong: %+v", turns[1])
	}
}

func TestSession_HistoryContextFeedsForward(t *testing.T) {
	e := New(fakeTools{})
	sess := e.NewSession(SessionOptions{})
	if _, err := sess.Run(context.Background(), &fakeModel{reply: "ans1"}, "q1"); err != nil {
		t.Fatal(err)
	}
	ctx := sess.historyContext()
	if !strings.Contains(ctx, "q1") || !strings.Contains(ctx, "ans1") {
		t.Errorf("history context missing prior turn: %q", ctx)
	}
}

func TestSession_HistoryLimit(t *testing.T) {
	e := New(fakeTools{})
	sess := e.NewSession(SessionOptions{HistoryLimit: 2})
	for i := 0; i < 5; i++ {
		if _, err := sess.Run(context.Background(), &fakeModel{reply: "a"}, "msg"); err != nil {
			t.Fatal(err)
		}
	}
	// Only the last 2 turns should appear in the context.
	ctx := sess.historyContext()
	if strings.Count(ctx, "[USER]") != 2 {
		t.Errorf("expected 2 turns in context, got: %q", ctx)
	}
}

func TestSession_ExportRestoreRoundTrip(t *testing.T) {
	e := New(fakeTools{})
	sess := e.NewSession(SessionOptions{SystemPrompt: "be terse", ConnectionID: "c9", MaxRows: 50, HistoryLimit: 4})
	if _, err := sess.Run(context.Background(), &fakeModel{reply: "hi"}, "hello"); err != nil {
		t.Fatal(err)
	}

	state := sess.Export()
	if state.ConnectionID != "c9" || state.SystemPrompt != "be terse" || state.MaxRows != 50 {
		t.Errorf("exported state lost options: %+v", state)
	}
	if len(state.Turns) != 1 {
		t.Fatalf("expected 1 turn in state, got %d", len(state.Turns))
	}

	restored := e.RestoreSession(state)
	if len(restored.Turns()) != 1 {
		t.Errorf("restored session lost turns")
	}
	// A new turn continues from restored history.
	if _, err := restored.Run(context.Background(), &fakeModel{reply: "again"}, "more"); err != nil {
		t.Fatal(err)
	}
	if len(restored.Turns()) != 2 {
		t.Errorf("restored session did not append new turn")
	}
	if restored.opts.SystemPrompt != "be terse" {
		t.Errorf("restored session lost system prompt")
	}
}

func TestRestoreSession_DefaultsHistoryLimit(t *testing.T) {
	e := New(fakeTools{})
	restored := e.RestoreSession(SessionState{}) // no HistoryLimit
	if restored.opts.HistoryLimit != 6 {
		t.Errorf("expected default history limit 6, got %d", restored.opts.HistoryLimit)
	}
}
