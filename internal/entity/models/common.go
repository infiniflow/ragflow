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

func GetThinkingAndAnswer(modelType *string, content *string) (*string, *string) {
	if content == nil {
		return nil, nil
	}

	switch NormalizeModelFamily(modelType) {
	case "qwen3":
		return extractThinkContent(content)
	}
	return nil, content
}

// NormalizeModelFamily normalizes provider-prefixed model class/name strings for shared response parsing.
func NormalizeModelFamily(modelType *string) string {
	if modelType == nil {
		return ""
	}

	family := strings.ToLower(strings.TrimSpace(*modelType))
	if family == "" {
		return ""
	}

	if slash := strings.LastIndex(family, "/"); slash >= 0 && slash < len(family)-1 {
		family = family[slash+1:]
	}

	if family == "qwen3" || strings.HasPrefix(family, "qwen3-") || strings.HasPrefix(family, "qwen3.") {
		return "qwen3"
	}

	if dash := strings.Index(family, "-"); dash >= 0 {
		family = family[:dash]
	}

	return family
}

func extractThinkContent(content *string) (*string, *string) {
	if content == nil {
		return nil, nil
	}

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
