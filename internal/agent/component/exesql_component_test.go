package component

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"

	"ragflow/internal/agent/runtime"
)

type exesqlInvokerStub struct {
	arguments string
	result    string
	err       error
}

func (s *exesqlInvokerStub) InvokableRun(_ context.Context, arguments string, _ ...einotool.Option) (string, error) {
	s.arguments = arguments
	return s.result, s.err
}

func TestExeSQLComponentResolvesConfiguredSQL(t *testing.T) {
	stub := &exesqlInvokerStub{
		result: `{"columns":["id","status"],"rows":[{"id":1,"status":"Completed"}]}`,
	}
	state := runtime.NewCanvasState("run", "task")
	state.SetVar("Agent:SparklyMooseDivide", "content", "SELECT id FROM orders WHERE status = 'Completed'")
	c := &exesqlComponent{
		inner: stub,
		sql:   "{Agent:SparklyMooseDivide@content}",
	}

	out, err := c.Invoke(runtime.WithState(context.Background(), state), map[string]any{
		"content": "this must not be used as SQL",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	var arguments map[string]any
	if err := json.Unmarshal([]byte(stub.arguments), &arguments); err != nil {
		t.Fatalf("decode tool arguments: %v", err)
	}
	if got := arguments["sql"]; got != "SELECT id FROM orders WHERE status = 'Completed'" {
		t.Fatalf("sql argument = %#v, want resolved SQL", got)
	}
	if _, exists := arguments["content"]; exists {
		t.Fatalf("tool arguments unexpectedly contain upstream content: %s", stub.arguments)
	}
	formalized, ok := out["formalized_content"].(string)
	if !ok || !strings.Contains(formalized, "Completed") {
		t.Fatalf("formalized_content = %#v, want rendered row", out["formalized_content"])
	}
	jsonResult, ok := out["json"].([]any)
	if !ok || len(jsonResult) != 1 {
		t.Fatalf("json = %#v, want one statement result", out["json"])
	}
}

func TestNewExeSQLComponentKeepsConfiguredSQL(t *testing.T) {
	component, err := newExeSQLComponent(map[string]any{
		"db_type":     "mysql",
		"database":    "demo",
		"username":    "root",
		"host":        "db.example.com",
		"port":        3306,
		"password":    "secret",
		"max_records": 100,
		"sql":         "{Agent:SparklyMooseDivide@content}",
	})
	if err != nil {
		t.Fatalf("newExeSQLComponent: %v", err)
	}
	exeSQL, ok := component.(*exesqlComponent)
	if !ok {
		t.Fatalf("component type = %T, want *exesqlComponent", component)
	}
	if exeSQL.sql != "{Agent:SparklyMooseDivide@content}" {
		t.Fatalf("configured SQL = %q", exeSQL.sql)
	}
}

func TestExeSQLComponentPreservesToolErrorAsCanvasOutput(t *testing.T) {
	stub := &exesqlInvokerStub{
		result: `{"_ERROR":"exesql: empty sql"}`,
		err:    errors.New("exesql: empty sql"),
	}
	c := &exesqlComponent{inner: stub, sql: ""}

	out, err := c.Invoke(context.Background(), map[string]any{"sql": ""})
	if err != nil {
		t.Fatalf("Invoke returned a hard error: %v", err)
	}
	if got := out["_ERROR"]; got != "exesql: empty sql" {
		t.Fatalf("_ERROR = %#v, want tool error", got)
	}
}
