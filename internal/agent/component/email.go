//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package component

import (
	"context"
	"encoding/json"
	"fmt"
	"ragflow/internal/agent/runtime"
	"strconv"
	"strings"

	agenttool "ragflow/internal/agent/tool"
)

const componentNameEmail = "Email"

// EmailComponent sends a canvas email node through the Go SMTP tool.
// It accepts the same DSL fields as the Python Email tool:
// smtp_server, smtp_port, email, smtp_username, password, sender_name,
// to_email, cc_email, content, and subject.
type EmailComponent struct {
	name         string
	smtpServer   string
	smtpPort     int
	email        string
	smtpUsername string
	password     string
	senderName   string
	toEmail      string
	ccEmail      string
	content      string
	subject      string
	tool         *agenttool.EmailTool
}

// NewEmailComponent constructs an Email component from DSL params.
func NewEmailComponent(params map[string]any) (Component, error) {
	smtpPort, err := emailIntParam(params, "smtp_port", 465)
	if err != nil {
		return nil, err
	}
	return &EmailComponent{
		name:         componentNameEmail,
		smtpServer:   emailStringParam(params, "smtp_server"),
		smtpPort:     smtpPort,
		email:        emailStringParam(params, "email"),
		smtpUsername: emailStringParam(params, "smtp_username"),
		password:     emailStringParam(params, "password"),
		senderName:   emailStringParam(params, "sender_name"),
		toEmail:      emailStringParam(params, "to_email"),
		ccEmail:      emailStringParam(params, "cc_email"),
		content:      emailStringParam(params, "content"),
		subject:      emailStringParam(params, "subject"),
		tool:         agenttool.NewEmailTool(),
	}, nil
}

// Name returns the registered component name.
func (e *EmailComponent) Name() string { return e.name }

// Invoke sends the email and returns success/_ERROR fields, matching the
// Python node's soft-failure behaviour.
func (e *EmailComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	toEmail := firstString(inputs, "to_email", e.toEmail)
	content := firstString(inputs, "content", e.content)
	subject := firstString(inputs, "subject", e.subject)
	ccEmail := firstString(inputs, "cc_email", e.ccEmail)

	state, _, _ := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	toEmail = runtime.ResolveTemplateForDisplay(toEmail, state)
	ccEmail = runtime.ResolveTemplateForDisplay(ccEmail, state)
	subject = runtime.ResolveTemplateForDisplay(subject, state)
	subject = stripEmailHeaderLineBreaks(subject)
	content = runtime.ResolveTemplateForDisplay(content, state)

	args := map[string]any{
		"smtp_host": e.smtpServer,
		"smtp_port": e.smtpPort,
		"username":  firstNonEmpty(e.smtpUsername, e.email),
		"password":  e.password,
		"from_addr": e.email,
		"to_addrs":  emailRecipients(toEmail, ccEmail),
		"subject":   subject,
		"body":      content,
	}
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return map[string]any{"success": false, "_ERROR": fmt.Sprintf("email: marshal arguments: %v", err)}, nil
	}

	out, err := e.tool.InvokableRun(ctx, string(argsJSON))
	if err != nil {
		return map[string]any{"success": false, "_ERROR": err.Error()}, nil
	}
	var env struct {
		OK    bool   `json:"ok"`
		Error string `json:"_ERROR"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		return map[string]any{"success": false, "_ERROR": fmt.Sprintf("email: parse result: %v", err)}, nil
	}
	if env.Error != "" {
		return map[string]any{"success": false, "_ERROR": env.Error}, nil
	}
	return map[string]any{"success": env.OK}, nil
}

// Stream mirrors Invoke as a single-chunk stream.
func (e *EmailComponent) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := e.Invoke(ctx, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

func (e *EmailComponent) GetInputForm() map[string]any {
	return map[string]any{
		"to_email": map[string]any{
			"name": "To",
			"type": "line",
		},
		"subject": map[string]any{
			"name": "Subject",
			"type": "line",
		},
		"cc_email": map[string]any{
			"name": "CC To",
			"type": "line",
		},
	}
}

// Inputs returns the DSL input surface.
func (e *EmailComponent) Inputs() map[string]string {
	return map[string]string{
		"to_email":      "Recipient email address list.",
		"cc_email":      "Optional CC recipient list.",
		"content":       "Email body.",
		"subject":       "Email subject.",
		"smtp_server":   "SMTP server hostname.",
		"smtp_port":     "SMTP server port.",
		"email":         "Sender email address.",
		"smtp_username": "SMTP username; defaults to sender email.",
		"password":      "SMTP password or authorization code.",
		"sender_name":   "Sender display name.",
	}
}

// Outputs returns the public output surface.
func (e *EmailComponent) Outputs() map[string]string {
	return map[string]string{
		"success": "Whether the email was sent successfully.",
		"_ERROR":  "SMTP error message when sending fails.",
	}
}

func emailStringParam(params map[string]any, key string) string {
	if v, ok := params[key].(string); ok {
		return v
	}
	return ""
}

func emailIntParam(params map[string]any, key string, fallback int) (int, error) {
	switch v := params[key].(type) {
	case nil:
		return fallback, nil
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	case json.Number:
		n, err := v.Int64()
		return int(n), err
	case string:
		if strings.TrimSpace(v) == "" {
			return fallback, nil
		}
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, &ParamError{Field: key, Reason: "must be an integer"}
		}
		return n, nil
	default:
		return 0, &ParamError{Field: key, Reason: "must be an integer"}
	}
}

func firstString(inputs map[string]any, key, fallback string) string {
	if v, ok := inputs[key].(string); ok {
		return v
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func emailRecipients(toEmail, ccEmail string) []string {
	recipients := splitEmailList(toEmail)
	recipients = append(recipients, splitEmailList(ccEmail)...)
	return recipients
}

func splitEmailList(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if s := strings.TrimSpace(part); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func stripEmailHeaderLineBreaks(value string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(value)
}

func init() {
	Register(componentNameEmail, NewEmailComponent)
}
