package models

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestEinoChatModelStreamFiltersDoneSentinel(t *testing.T) {
	modelName := "chat"
	driver := &streamSentinelDriver{captureToolDriver: &captureToolDriver{}}
	var callbacks []string
	base := NewChatModel(driver, &modelName, &APIConfig{})
	model := NewEinoChatModel(base, &ChatConfig{
		StreamCallback: func(content, _ string) {
			if content != "" {
				callbacks = append(callbacks, content)
			}
		},
	})

	stream, err := model.Stream(context.Background(), []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var messages []string
	for {
		msg, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			t.Fatalf("stream.Recv: %v", recvErr)
		}
		if msg != nil {
			messages = append(messages, msg.Content)
		}
	}

	if len(messages) != 2 || messages[0] != "answer" || messages[1] != "DONE!" {
		t.Fatalf("stream messages = %#v, want [answer DONE!]", messages)
	}
	if len(callbacks) != 2 || callbacks[0] != "answer" || callbacks[1] != "DONE!" {
		t.Fatalf("stream callbacks = %#v, want [answer DONE!]", callbacks)
	}
}

func TestEinoChatModelGenerateSendsBoundTools(t *testing.T) {
	apiKey := "key"
	modelName := "chat"
	driver := &captureToolDriver{
		resp: &ChatResponse{
			ToolCalls: []map[string]interface{}{
				{
					"id":   "call-1",
					"type": "function",
					"function": map[string]interface{}{
						"name":      "search_my_dateset",
						"arguments": `{"query":"hello"}`,
					},
				},
			},
		},
	}
	base := NewChatModel(driver, &modelName, &APIConfig{ApiKey: &apiKey})
	model := NewEinoChatModel(base, nil)
	bound, err := model.WithTools([]*schema.ToolInfo{
		{
			Name: "search_my_dateset",
			Desc: "Search datasets.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"query": &schema.ParameterInfo{Type: schema.String, Required: true},
			}),
		},
	})
	if err != nil {
		t.Fatalf("WithTools: %v", err)
	}

	msg, err := bound.Generate(context.Background(), []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if driver.lastConfig == nil || driver.lastConfig.Tools == nil {
		t.Fatal("Generate did not send tools to driver")
	}
	tools, ok := driver.lastConfig.Tools.([]map[string]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("driver tools = %#v, want one OpenAI-style tool", driver.lastConfig.Tools)
	}
	fn, _ := tools[0]["function"].(map[string]any)
	if fn["name"] != "search_my_dateset" {
		t.Fatalf("tool function name = %#v, want search_my_dateset", fn["name"])
	}
	if driver.lastConfig.ToolChoice == nil || *driver.lastConfig.ToolChoice != "auto" {
		t.Fatalf("ToolChoice = %#v, want auto", driver.lastConfig.ToolChoice)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("msg.ToolCalls len = %d, want 1", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].Function.Name != "search_my_dateset" || msg.ToolCalls[0].Function.Arguments != `{"query":"hello"}` {
		t.Fatalf("tool call = %#v, want search_my_dateset query call", msg.ToolCalls[0])
	}
}

func TestEinoChatModelStreamWithToolsYieldsToolCalls(t *testing.T) {
	apiKey := "key"
	modelName := "chat"
	driver := &captureToolDriver{
		resp: &ChatResponse{
			ToolCalls: []map[string]interface{}{
				{
					"id":   "call-1",
					"type": "function",
					"function": map[string]interface{}{
						"name":      "search_my_dateset",
						"arguments": `{"query":"hello"}`,
					},
				},
			},
		},
	}
	base := NewChatModel(driver, &modelName, &APIConfig{ApiKey: &apiKey})
	model := NewEinoChatModel(base, nil)
	bound, err := model.WithTools([]*schema.ToolInfo{
		{
			Name: "search_my_dateset",
			Desc: "Search datasets.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"query": &schema.ParameterInfo{Type: schema.String, Required: true},
			}),
		},
	})
	if err != nil {
		t.Fatalf("WithTools: %v", err)
	}

	stream, err := bound.Stream(context.Background(), []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	msg, err := stream.Recv()
	if err != nil {
		t.Fatalf("stream.Recv: %v", err)
	}
	if msg == nil {
		t.Fatal("stream ended before yielding message")
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Name != "search_my_dateset" {
		t.Fatalf("stream message tool calls = %#v, want search_my_dateset", msg.ToolCalls)
	}
}

func TestToInternalMessagesPreservesToolMessages(t *testing.T) {
	internal := toInternalMessages([]*schema.Message{
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "search_my_dateset",
					Arguments: `{"query":"hello"}`,
				},
			}},
		},
		{
			Role:       schema.Tool,
			Content:    `{"formalized_content":"answer"}`,
			ToolCallID: "call-1",
		},
	})
	if len(internal) != 2 {
		t.Fatalf("len(internal) = %d, want 2", len(internal))
	}
	if len(internal[0].ToolCalls) != 1 {
		t.Fatalf("assistant ToolCalls = %#v, want one tool call", internal[0].ToolCalls)
	}
	if internal[1].ToolCallID != "call-1" || internal[1].Role != "tool" {
		t.Fatalf("tool message = %#v, want tool role with call id", internal[1])
	}
}

type captureToolDriver struct {
	resp       *ChatResponse
	lastConfig *ChatConfig
}

type streamSentinelDriver struct {
	*captureToolDriver
}

func (d *streamSentinelDriver) ChatStreamlyWithSender(_ string, _ []Message, _ *APIConfig, _ *ChatConfig, sender func(*string, *string) error) error {
	answer := "answer"
	if err := sender(&answer, nil); err != nil {
		return err
	}
	visibleDone := "DONE!"
	if err := sender(&visibleDone, nil); err != nil {
		return err
	}
	done := "[DONE]"
	return sender(&done, nil)
}

func (d *captureToolDriver) NewInstance(baseURL map[string]string) ModelDriver { return d }
func (d *captureToolDriver) Name() string                                      { return "capture" }
func (d *captureToolDriver) ChatWithMessages(_ string, _ []Message, _ *APIConfig, cfg *ChatConfig) (*ChatResponse, error) {
	d.lastConfig = cfg
	return d.resp, nil
}
func (d *captureToolDriver) ChatStreamlyWithSender(_ string, _ []Message, _ *APIConfig, _ *ChatConfig, _ func(*string, *string) error) error {
	return nil
}
func (d *captureToolDriver) Embed(_ *string, _ []string, _ *APIConfig, _ *EmbeddingConfig) ([]EmbeddingData, error) {
	return nil, nil
}
func (d *captureToolDriver) Rerank(_ *string, _ string, _ []string, _ *APIConfig, _ *RerankConfig) (*RerankResponse, error) {
	return nil, nil
}
func (d *captureToolDriver) TranscribeAudio(_ *string, _ *string, _ *APIConfig, _ *ASRConfig) (*ASRResponse, error) {
	return nil, nil
}
func (d *captureToolDriver) TranscribeAudioWithSender(_ *string, _ *string, _ *APIConfig, _ *ASRConfig, _ func(*string, *string) error) error {
	return nil
}
func (d *captureToolDriver) AudioSpeech(_ *string, _ *string, _ *APIConfig, _ *TTSConfig) (*TTSResponse, error) {
	return nil, nil
}
func (d *captureToolDriver) AudioSpeechWithSender(_ *string, _ *string, _ *APIConfig, _ *TTSConfig, _ func(*string, *string) error) error {
	return nil
}
func (d *captureToolDriver) OCRFile(_ *string, _ []byte, _ *string, _ *APIConfig, _ *OCRConfig) (*OCRFileResponse, error) {
	return nil, nil
}
func (d *captureToolDriver) ParseFile(_ *string, _ []byte, _ *string, _ *APIConfig, _ *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, nil
}
func (d *captureToolDriver) ListModels(_ *APIConfig) ([]ListModelResponse, error) {
	return nil, nil
}
func (d *captureToolDriver) Balance(_ *APIConfig) (map[string]interface{}, error) {
	return nil, nil
}
func (d *captureToolDriver) CheckConnection(_ *APIConfig) error { return nil }
func (d *captureToolDriver) ListTasks(_ *APIConfig) ([]ListTaskStatus, error) {
	return nil, nil
}
func (d *captureToolDriver) ShowTask(_ string, _ *APIConfig) (*TaskResponse, error) {
	return nil, nil
}
