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
	"regexp"
	"strings"
)

var (
	completeThinkBlockPattern = regexp.MustCompile(`(?s)<think>.*?</think>`)
	danglingThinkBlockPattern = regexp.MustCompile(`(?s)<think>.*$`)
	standaloneThinkTagPattern = regexp.MustCompile(`</?think>`)
	excessNewlinePattern      = regexp.MustCompile(`\n{3,}`)
)

func stripThinking(content string) string {
	if content == "" {
		return content
	}
	cleaned := completeThinkBlockPattern.ReplaceAllString(content, "")
	cleaned = danglingThinkBlockPattern.ReplaceAllString(cleaned, "")
	cleaned = standaloneThinkTagPattern.ReplaceAllString(cleaned, "")
	cleaned = excessNewlinePattern.ReplaceAllString(cleaned, "\n\n")
	return strings.TrimSpace(cleaned)
}
