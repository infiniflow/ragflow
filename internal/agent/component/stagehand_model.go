package component

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/json/v2"
	"fmt"
	"net"
	"net/http"
	"time"

	"ragflow/internal/entity/models"
)

// Stagehand speaks OpenAI on this private connection; the selected Go driver
// owns the actual provider protocol, credentials, endpoints and response parsing.
func startStagehandModelServer(ctx context.Context, model *models.ChatModel) (baseURL, token string, closeServer func(), err error) {
	if model == nil || model.ModelDriver == nil {
		return "", "", nil, fmt.Errorf("stagehand model: resolved chat model is required")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", "", nil, fmt.Errorf("stagehand model: listen: %w", err)
	}
	token = rand.Text()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fail := func(status int, err error) {
			w.WriteHeader(status)
			_ = json.MarshalWrite(w, map[string]any{"error": map[string]any{"message": err.Error(), "type": "model_error"}})
		}
		if !hmac.Equal([]byte(request.Header.Get("Authorization")), []byte("Bearer "+token)) {
			fail(http.StatusUnauthorized, fmt.Errorf("invalid Stagehand model token"))
			return
		}
		var input struct {
			Messages            []models.Message `json:"messages"`
			Stream              bool             `json:"stream"`
			Temperature         *float64         `json:"temperature"`
			TopP                *float64         `json:"top_p"`
			MaxTokens           *int             `json:"max_tokens"`
			MaxCompletionTokens *int             `json:"max_completion_tokens"`
			Stop                any              `json:"stop"`
			Tools               []map[string]any `json:"tools"`
			ToolChoice          any              `json:"tool_choice"`
			ResponseFormat      *struct {
				Type       string `json:"type"`
				JSONSchema struct {
					Schema map[string]any `json:"schema"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if err := json.UnmarshalRead(http.MaxBytesReader(w, request.Body, 64<<20), &input); err != nil {
			fail(http.StatusBadRequest, err)
			return
		}
		if input.Stream || len(input.Messages) == 0 {
			fail(http.StatusBadRequest, fmt.Errorf("Stagehand model requires non-streaming messages"))
			return
		}
		config := &models.ChatConfig{Temperature: input.Temperature, TopP: input.TopP, MaxTokens: input.MaxTokens}
		if config.MaxTokens == nil {
			config.MaxTokens = input.MaxCompletionTokens
		}
		if len(input.Tools) > 0 {
			config.Tools = input.Tools
		}
		if choice, ok := input.ToolChoice.(string); ok {
			config.ToolChoice = &choice
		} else if input.ToolChoice != nil {
			config.ToolChoice = new("required")
			config.ToolChoiceValue = input.ToolChoice
		}
		if input.Stop != nil {
			var stop []string
			switch value := input.Stop.(type) {
			case string:
				stop = []string{value}
			case []any:
				for _, item := range value {
					text, ok := item.(string)
					if !ok {
						fail(http.StatusBadRequest, fmt.Errorf("invalid stop sequence"))
						return
					}
					stop = append(stop, text)
				}
			default:
				fail(http.StatusBadRequest, fmt.Errorf("invalid stop sequence"))
				return
			}
			config.Stop = &stop
		}
		structured := false
		if format := input.ResponseFormat; format != nil && format.Type != "text" {
			if len(input.Tools) > 0 {
				fail(http.StatusBadRequest, fmt.Errorf("combined structured output and tools are unsupported"))
				return
			}
			schema := format.JSONSchema.Schema
			switch format.Type {
			case "json_object":
				schema = map[string]any{"type": "object"}
			case "json_schema":
				if len(schema) == 0 {
					fail(http.StatusBadRequest, fmt.Errorf("structured output schema is required"))
					return
				}
			default:
				fail(http.StatusBadRequest, fmt.Errorf("unsupported response format %q", format.Type))
				return
			}
			// Function tools are the shared provider contract for structured output.
			config.Tools = []map[string]any{{"type": "function", "function": map[string]any{"name": "ragflow_structured_output", "description": "Return the requested structured result.", "parameters": schema}}}
			config.ToolChoice = new("required")
			config.ToolChoiceValue = map[string]any{"type": "function", "function": map[string]any{"name": "ragflow_structured_output"}}
			structured = true
		}
		result, err := model.ChatWithMessages(request.Context(), input.Messages, config, nil)
		if err != nil {
			fail(http.StatusBadGateway, err)
			return
		}
		if result == nil {
			fail(http.StatusBadGateway, fmt.Errorf("provider returned no response"))
			return
		}
		content := result.Answer
		calls := result.ToolCalls
		if structured {
			if len(calls) != 1 {
				fail(http.StatusBadGateway, fmt.Errorf("provider did not return the structured output tool"))
				return
			}
			function, _ := calls[0]["function"].(map[string]any)
			arguments, ok := function["arguments"].(string)
			var value any
			if !ok || function["name"] != "ragflow_structured_output" || json.Unmarshal([]byte(arguments), &value) != nil {
				fail(http.StatusBadGateway, fmt.Errorf("provider returned invalid structured output"))
				return
			}
			content = &arguments
			calls = nil
		}
		message := map[string]any{"role": "assistant", "content": content}
		finishReason := "stop"
		if len(calls) > 0 {
			message["tool_calls"] = calls
			finishReason = "tool_calls"
		}
		if result.ReasonContent != nil {
			message["reasoning_content"] = *result.ReasonContent
		}
		response := map[string]any{"id": "chatcmpl-stagehand", "object": "chat.completion", "created": time.Now().Unix(), "model": "ragflow", "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finishReason}}}
		if result.Usage != nil {
			response["usage"] = result.Usage
		}
		_ = json.MarshalWrite(w, response)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second, BaseContext: func(net.Listener) context.Context { return ctx }}
	go func() { _ = server.Serve(listener) }()
	stop := context.AfterFunc(ctx, func() { _ = server.Close() })
	return "http://" + listener.Addr().String() + "/v1", token, func() { stop(); _ = server.Close() }, nil
}
