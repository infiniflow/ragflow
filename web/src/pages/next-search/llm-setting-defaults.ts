/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

// Mirrors LLM_SETTING_DEFAULTS in api/db/services/llm_service.py: when a
// generation parameter was never saved, the backend resolves it to these
// defaults. Falling back to 0 in the form would persist 0 as an explicit
// value and override the backend defaults on the next save (e.g. top_p 0
// collapses nucleus sampling).
export const llmSettingDefaults = {
  temperature: 0.1,
  top_p: 0.3,
  frequency_penalty: 0.7,
  presence_penalty: 0.4,
} as const;

type LlmSettingNumericFields = {
  temperature?: number | null;
  top_p?: number | null;
  frequency_penalty?: number | null;
  presence_penalty?: number | null;
};

/**
 * Fill the four generation parameters for the form's initial values. A stored
 * 0 is a deliberate choice and must be kept; only unset values fall back to
 * the backend defaults.
 */
export const resolveInitialLlmSetting = (
  llm_setting?: LlmSettingNumericFields | null,
) => ({
  temperature: llm_setting?.temperature ?? llmSettingDefaults.temperature,
  top_p: llm_setting?.top_p ?? llmSettingDefaults.top_p,
  frequency_penalty:
    llm_setting?.frequency_penalty ?? llmSettingDefaults.frequency_penalty,
  presence_penalty:
    llm_setting?.presence_penalty ?? llmSettingDefaults.presence_penalty,
});
