// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package service

import "errors"

// CompletionHistoryRequest rejects client-supplied history for completion APIs.
type CompletionHistoryRequest struct {
	PassAllHistoryMessages bool `json:"pass_all_history_messages,omitempty"`
	PassAllHistory         bool `json:"pass_all_history,omitempty"`
}

func (r CompletionHistoryRequest) ValidateHistory() error {
	if r.PassAllHistoryMessages || r.PassAllHistory {
		return errors.New("does not support")
	}
	return nil
}
