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

import { PromptConfig } from '@/interfaces/database/chat';

/**
 * Chat assistants created through the REST API / SDK may carry a
 * `prompt_config` that omits keys `useChatSettingSchema` marks as required
 * (`keyword`, `tts`, `refine_multiturn`, ...). Such a chat can never pass
 * form validation, so the settings panel's Save button silently does nothing —
 * the user has no way to change the model, prompt or retrieval settings.
 *
 * These defaults backfill only the missing keys, leaving every value the chat
 * already has untouched, so an API-created chat becomes editable.
 *
 * `system` is deliberately left out: the schema requires a non-empty value,
 * and inventing a prompt would be worse than letting validation point at the
 * empty field.
 */
export const promptConfigDefaults: Partial<PromptConfig> = {
  quote: true,
  keyword: false,
  tts: false,
  refine_multiturn: true,
  parameters: [],
  reasoning: false,
  cross_languages: [],
};

export function withPromptConfigDefaults(
  promptConfig?: Partial<PromptConfig> | null,
): Partial<PromptConfig> {
  return { ...promptConfigDefaults, ...(promptConfig ?? {}) };
}
