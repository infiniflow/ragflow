package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"io"
	"net/http"
	"strings"
	"time"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/common"
	"ragflow/internal/dao"
)

// ChatChannelRESTClient executes channel turns through the authenticated API.
type ChatChannelRESTClient struct {
	baseURL string
	client  *http.Client
}

func NewChatChannelRESTClient(baseURL string) *ChatChannelRESTClient {
	return &ChatChannelRESTClient{baseURL: strings.TrimRight(baseURL, "/"), client: &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (c *ChatChannelRESTClient) HandleIncomingMessage(ctx context.Context, msg ChatChannelIncomingMessage, onDelta func(string)) (answer string, err error) {
	if strings.TrimSpace(msg.Text) == "" {
		return "", nil
	}
	channel, err := dao.NewChatChannel().GetByID(ctx, dao.DB, msg.AccountID)
	if dao.IsNotFoundErr(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if channel.Status != 1 || (channel.ChatID == nil && channel.AgentID == nil) {
		return "", nil
	}
	key, err := chatChannelExecutionKey(ctx, channel)
	if err != nil {
		return "", err
	}
	body := map[string]any{"question": msg.Text, "stream": true, "quote": false, "channel_context": map[string]string{"channel_id": channel.ID, "chat_id": msg.ChatID}}
	agent := channel.AgentID != nil
	defer func() {
		if !agent && err != nil {
			common.Error("chat channel completion failed", err, zap.String("channel_id", channel.ID))
			answer, err = "Sorry, I couldn't process your message right now.", nil
		}
	}()
	path := "/api/v1/chat/completions"
	if agent {
		path = "/api/v1/agents/chat/completions"
		body["agent_id"] = *channel.AgentID
	} else {
		body["chat_id"] = *channel.ChatID
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	response, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("channel completion API is unavailable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("channel completion API rejected the request (HTTP %d)", response.StatusCode)
	}
	if !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		var failure struct {
			Code int `json:"code"`
		}
		payload, readErr := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		if readErr != nil {
			return "", fmt.Errorf("read channel completion rejection: %w", readErr)
		}
		if err := json.Unmarshal(payload, &failure); err != nil {
			return "", errors.New("channel completion API returned an invalid response")
		}
		return "", fmt.Errorf("channel completion API rejected the request (code %d)", failure.Code)
	}
	return consumeChatChannelRESTStream(response.Body, agent, onDelta)
}

func consumeChatChannelRESTStream(reader io.Reader, agent bool, onDelta func(string)) (string, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 16*1024*1024)
	var answer strings.Builder
	thinking := false
	emit := func() {
		if onDelta != nil {
			onDelta(stripChannelCitations(answer.String(), true))
		}
	}
	var data []string
	done := false
	consume := func(payload string) error {
		if payload == "[DONE]" {
			done = true
			return nil
		}
		var frame struct {
			Code  int            `json:"code"`
			Event string         `json:"event"`
			Data  jsontext.Value `json:"data"`
		}
		if err := json.Unmarshal([]byte(payload), &frame); err != nil {
			return errors.New("invalid channel completion stream")
		}
		if frame.Code != 0 || frame.Event == "error" {
			return errors.New("channel workflow execution failed")
		}
		if frame.Event == "cancelled" {
			return context.Canceled
		}
		if string(frame.Data) == "true" {
			done = true
			return nil
		}
		if agent {
			switch frame.Event {
			case "message":
				var msg canvas.MessageEvent
				if err := json.Unmarshal(frame.Data, &msg); err != nil {
					return errors.New("invalid channel message")
				}
				if msg.StartToThink {
					thinking = true
				}
				if msg.EndToThink {
					thinking = false
					return nil
				}
				if !thinking {
					answer.WriteString(msg.Content)
					emit()
				}
			case "waiting_for_user":
				var waiting canvas.WaitingForUserEvent
				if err := json.Unmarshal(frame.Data, &waiting); err != nil {
					return errors.New("invalid channel input request")
				}
				if len(waiting.Inputs) > 1 {
					answer.Reset()
					answer.WriteString("This workflow requires structured input. Please continue in RAGFlow.")
				} else {
					if answer.Len() > 0 && waiting.Tips != "" {
						answer.WriteString("\n")
					}
					answer.WriteString(waiting.Tips)
					if answer.Len() == 0 {
						answer.WriteString("Please reply to continue.")
					}
				}
				emit()
			}
		} else {
			var chunk struct {
				Answer string `json:"answer"`
				Final  bool   `json:"final"`
			}
			if err := json.Unmarshal(frame.Data, &chunk); err != nil {
				return errors.New("invalid channel answer")
			}
			if chunk.Final {
				thinking = false
				answer.Reset()
			}
			answer.WriteString(stripChannelThink(chunk.Answer, &thinking))
			emit()
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if len(data) > 0 {
				if err := consume(strings.Join(data, "\n")); err != nil {
					return "", err
				}
				data = nil
				if done {
					return stripChannelCitations(answer.String(), false), nil
				}
			}
		} else if value, ok := strings.CutPrefix(line, "data:"); ok {
			data = append(data, strings.TrimPrefix(value, " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("channel completion stream interrupted: %w", err)
	}
	return "", errors.New("channel completion stream ended before completion")
}
