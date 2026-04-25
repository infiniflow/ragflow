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

package models

import "strings"

func GetThinkingAndAnswer(modelSeries *string, content *string) (*string, *string) {
	switch *modelSeries {
	case "qwen3":
		return extractThinkContent(content)
	}
	return nil, content
}

func extractThinkContent(content *string) (*string, *string) {
	startTag := "<think>"
	endTag := "</think>"

	startIdx := strings.Index(*content, startTag)
	endIdx := strings.Index(*content, endTag)

	if startIdx == -1 || endIdx == -1 || endIdx <= startIdx {
		return nil, content
	}

	thinking := (*content)[startIdx+len(startTag) : endIdx]
	answer := (*content)[endIdx+len(endTag):]

	thinking = strings.TrimLeft(thinking, "\n")
	answer = strings.TrimLeft(answer, "\n")

	return &thinking, &answer
}
