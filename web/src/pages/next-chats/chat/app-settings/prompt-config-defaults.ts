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
 * (`keyword`, `tts`, `system`, `refine_multiturn`, ...). Such a chat can never
 * pass form validation, so the settings panel's Save button silently does
 * nothing — the user has no way to change the model, prompt or retrieval
 * settings.
 *
 * These defaults backfill only the missing keys, leaving every value the chat
 * already has untouched, so an API-created chat becomes editable.
 *
 * `system` is required by the schema and must be non-empty, so it cannot simply
 * be skipped. It falls back to the same prompt the UI puts on a newly created
 * assistant, which keeps the save valid without inventing a prompt of our own;
 * a chat that already has one never sees this value.
 */
export function promptConfigDefaults(
  systemPrompt: string,
): Partial<PromptConfig> {
  return {
    quote: true,
    keyword: false,
    tts: false,
    system: systemPrompt,
    refine_multiturn: true,
    parameters: [],
    reasoning: false,
    cross_languages: [],
  };
}

export function withPromptConfigDefaults(
  promptConfig: Partial<PromptConfig> | null | undefined,
  systemPrompt: string,
): Partial<PromptConfig> {
  const merged: Partial<PromptConfig> = { ...promptConfigDefaults(systemPrompt) };
  for (const [key, value] of Object.entries(promptConfig ?? {})) {
    // A key that is present but `undefined` must not clobber its default:
    // spreading it verbatim is exactly how the backfill would silently fail.
    if (value !== undefined) {
      (merged as Record<string, unknown>)[key] = value;
    }
  }
  return merged;
}

/**
 * Builds the `prompt_config` sent on save.
 *
 * `promptConfigSchema` only declares the fields the form renders, and zod
 * strips everything else — so `use_kg` and `toc_enhance` are gone by the time
 * the submit handler runs. Because the PUT replaces the whole object, sending
 * the parsed values alone would silently delete them. Starting from the
 * persisted config keeps undeclared fields, while the submitted values still
 * win for every field the form does own.
 */
export function mergePromptConfigForSubmit(
  persisted?: Partial<PromptConfig> | null,
  submitted?: Partial<PromptConfig> | null,
): Partial<PromptConfig> {
  return { ...(persisted ?? {}), ...(submitted ?? {}) };
}
