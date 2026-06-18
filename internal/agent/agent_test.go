package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// fakeModel is a tool-calling chat model stub that returns a fixed answer with
// no tool calls, so the ReAct loop terminates immediately.
type fakeModel struct{ reply string }

func (f *fakeModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return schema.AssistantMessage(f.reply, nil), nil
}

func (f *fakeModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		sw.Send(schema.AssistantMessage(f.reply, nil), nil)
		sw.Close()
	}()
	return sr, nil
}

func (f *fakeModel) WithTools(_ []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return f, nil
}

type fakeTools struct{}

func (fakeTools) Schema(_ context.Context, _ string) (string, error) {
	return "table users(id int, name text)", nil
}

func (fakeTools) RunSQL(_ context.Context, _, _ string) (*SQLResult, error) {
	return &SQLResult{
		Columns:  []string{"n"},
		Rows:     []map[string]interface{}{{"n": 1}},
		RowCount: 1,
	}, nil
}

func (fakeTools) SearchMemory(_ context.Context, _ string, _ int) (string, error) {
	return "", nil
}

func TestEngineRunReturnsAnswer(t *testing.T) {
	e := New(fakeTools{})
	out, err := e.Run(context.Background(), &fakeModel{reply: "Hello from the agent"}, Input{
		Message:      "hi",
		ConnectionID: "c1",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Answer != "Hello from the agent" {
		t.Fatalf("answer = %q, want %q", out.Answer, "Hello from the agent")
	}
}

func TestRunSQLToolRecordsState(t *testing.T) {
	e := New(fakeTools{})
	rs := &runState{}
	tools, err := e.buildTools(rs, "c1", 50)
	if err != nil {
		t.Fatalf("buildTools: %v", err)
	}

	var runSQL tool.InvokableTool
	for _, tl := range tools {
		info, err := tl.Info(context.Background())
		if err != nil {
			t.Fatalf("Info: %v", err)
		}
		if info.Name == "run_sql" {
			runSQL = tl.(tool.InvokableTool)
		}
	}
	if runSQL == nil {
		t.Fatal("run_sql tool not found")
	}

	res, err := runSQL.InvokableRun(context.Background(), `{"sql":"SELECT 1"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(res, "Returned 1 row") {
		t.Fatalf("tool result = %q, want it to mention 1 row", res)
	}
	if rs.executedSQL != "SELECT 1" {
		t.Fatalf("executedSQL = %q, want SELECT 1", rs.executedSQL)
	}
	if rs.lastResult == nil || rs.lastResult.RowCount != 1 {
		t.Fatalf("lastResult not recorded correctly: %+v", rs.lastResult)
	}
}
