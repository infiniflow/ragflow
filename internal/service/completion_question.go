// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package service

import "errors"

// ResolveStoreHistoryMessages defaults to storage; only an explicit false is supported.
func ResolveStoreHistoryMessages(kwargs map[string]interface{}) (bool, error) {
	value, exists := kwargs["store_history_messages"]
	if !exists {
		return true, nil
	}
	if store, ok := value.(bool); !ok || store {
		return false, errors.New("`store_history_messages` only supports false.")
	}
	return false, nil
}

// ResolveCompletionQuestion picks explicit question/query text before the final user message.
func ResolveCompletionQuestion(question, query string, messages []map[string]interface{}) (string, error) {
	if question != "" {
		return question, nil
	}
	if query != "" {
		return query, nil
	}
	if len(messages) == 0 {
		return "", nil
	}
	message := messages[len(messages)-1]
	if message["role"] != "user" {
		return "", errors.New("The last content of this conversation is not from user.")
	}
	return NormalizeOpenAIMessageContent(message["content"])
}
