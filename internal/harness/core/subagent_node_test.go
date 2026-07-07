package core

import (
	"context"
	"testing"

	"ragflow/internal/harness/core/schema"
	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/graph"
)

// TestSubAgentNode_Simple verifies a basic sub-agent node in a StateGraph.
func TestSubAgentNode_Simple(t *testing.T) {
	m := &mockModel{}
	m.addResp("sub-agent response")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: m}).WithName("worker")

	sg := graph.NewStateGraph(map[string]interface{}{"Messages": []interface{}{}})
	sg.AddChannel("Messages", channels.NewLastValue([]interface{}{}))
	node := NewSubAgentNode(agent)
	sg.AddNode("worker", node)
	sg.AddEdge(constants.Start, "worker")
	sg.AddEdge("worker", constants.End)

	cg, err := sg.Compile(graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	result, err := cg.Invoke(context.Background(), map[string]interface{}{
		"Messages": []interface{}{schema.UserMessage("hello from sub-agent test")},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	_ = result
	t.Logf("sub-agent node result: %T", result)
}

// TestSubAgentNode_SequentialChain verifies two sub-agent nodes in sequence.
func TestSubAgentNode_SequentialChain(t *testing.T) {
	m1 := &mockModel{}
	m1.addResp("agent one")
	m2 := &mockModel{}
	m2.addResp("agent two")

	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1}).WithName("agent_a")
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2}).WithName("agent_b")

	sg := graph.NewStateGraph(map[string]interface{}{"Messages": []interface{}{}})
	sg.AddChannel("Messages", channels.NewLastValue([]interface{}{}))
	sg.AddNode("agent_a", NewSubAgentNode(a1))
	sg.AddNode("agent_b", NewSubAgentNode(a2))
	sg.AddEdge(constants.Start, "agent_a")
	sg.AddEdge("agent_a", "agent_b")
	sg.AddEdge("agent_b", constants.End)

	cg, err := sg.Compile(graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	_, err = cg.Invoke(context.Background(), map[string]interface{}{
		"Messages": []interface{}{schema.UserMessage("chain test")},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	t.Log("sequential sub-agent chain completed")
}

// TestSubAgentNode_WithFieldMapping verifies field-level input/output projection.
func TestSubAgentNode_WithFieldMapping(t *testing.T) {
	m := &mockModel{}
	m.addResp("projected result")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: m}).WithName("projector")

	sg := graph.NewStateGraph(map[string]interface{}{"query": "", "response": "", "Messages": []interface{}{}})
	sg.AddChannel("Messages", channels.NewLastValue([]interface{}{}))
	sg.AddChannel("query", channels.NewLastValue(""))
	sg.AddChannel("response", channels.NewLastValue(""))
	node := NewSubAgentNode(agent,
		WithSubAgentInput("query", "input"),
		WithSubAgentOutput("response", "response"),
	)
	sg.AddNode("projector", node)
	sg.AddEdge(constants.Start, "projector")
	sg.AddEdge("projector", constants.End)

	cg, err := sg.Compile(graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	result, err := cg.Invoke(context.Background(), map[string]interface{}{
		"query":    "what is go?",
		"response": "",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	st, ok := result.(map[string]interface{})
	if !ok {
		t.Fatal("expected map result")
	}
	resp, ok := st["response"].(string)
	if !ok || resp == "" {
		t.Error("expected response field to be populated (OutputMapping should project agent output to state)")
	}
	t.Logf("sub-agent with field mapping: response=%q", resp)
}

// TestSubAgentNode_BuilderCompile verifies SubAgentGraphBuilder compilation
// with manual edge wiring.
func TestSubAgentNode_BuilderCompile(t *testing.T) {
	m := &mockModel{}
	m.addResp("builder test")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: m}).WithName("builder_agent")

	sg := graph.NewStateGraph(map[string]interface{}{"Messages": []interface{}{}})
	sg.AddNode("node1", NewSubAgentNode(agent))
	sg.AddEdge(constants.Start, "node1")
	sg.AddEdge("node1", constants.End)

	cg, err := sg.Compile(graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if cg == nil {
		t.Fatal("expected non-nil compiled graph")
	}
	t.Log("builder compile passed")
}

// TestSubAgentNode_WithSubAgentName verifies name override.
func TestSubAgentNode_WithSubAgentName(t *testing.T) {
	m := &mockModel{}
	m.addResp("named agent")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: m}).WithName("original_name")

	sg := graph.NewStateGraph(map[string]interface{}{"Messages": []interface{}{}})
	sg.AddChannel("Messages", channels.NewLastValue([]interface{}{}))
	node := NewSubAgentNode(agent, WithSubAgentName("custom_name"))
	sg.AddNode("custom_name", node)
	sg.AddEdge(constants.Start, "custom_name")
	sg.AddEdge("custom_name", constants.End)

	cg, err := sg.Compile(graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	_, err = cg.Invoke(context.Background(), map[string]interface{}{
		"Messages": []interface{}{schema.UserMessage("name test")},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	t.Log("named sub-agent node completed")
}

// TestSubAgentNode_CustomExtractor verifies custom input extractor.
func TestSubAgentNode_CustomExtractor(t *testing.T) {
	m := &mockModel{}
	m.addResp("custom extractor ok")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: m}).WithName("extractor_test")

	sg := graph.NewStateGraph(map[string]interface{}{"data": "", "Messages": []interface{}{}})
	sg.AddChannel("Messages", channels.NewLastValue([]interface{}{}))
	sg.AddChannel("data", channels.NewLastValue(""))
	node := NewSubAgentNode(agent,
		WithSubAgentExtractor(func(ctx context.Context, state interface{}) (*AgentInput, error) {
			return &AgentInput{
				Messages: []*schema.Message{schema.UserMessage("custom input")},
			}, nil
		}),
	)
	sg.AddNode("extractor", node)
	sg.AddEdge(constants.Start, "extractor")
	sg.AddEdge("extractor", constants.End)

	cg, err := sg.Compile(graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	_, err = cg.Invoke(context.Background(), map[string]interface{}{
		"data": "some data",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	t.Log("custom extractor sub-agent completed")
}
