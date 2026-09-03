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

import "strings"

const defaultVisionLanguage = "English"

func normalizeVisionLanguage(language string) string {
	language = strings.TrimSpace(language)
	if language == "" {
		return defaultVisionLanguage
	}
	return language
}

func resolveVisionLanguage(inputs map[string]any, fallback string) string {
	if language := strings.TrimSpace(getStringOr(inputs, "lang", "")); language != "" {
		return language
	}
	return normalizeVisionLanguage(fallback)
}

func renderFigureVisionLanguage(prompt, language string) string {
	language = normalizeVisionLanguage(language)
	prompt = strings.ReplaceAll(prompt, "{{ language }}", language)
	return strings.ReplaceAll(prompt, "{{language}}", language)
}

func defaultImageVisionPrompt(language string) string {
	return "Describe this image in detail. Respond in " + normalizeVisionLanguage(language) + ". Preserve all visible text in its original language; do not translate it."
}
