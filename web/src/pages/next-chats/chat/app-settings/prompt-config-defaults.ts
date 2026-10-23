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
 * The `prompt_config` a newly created assistant starts with.
 *
 * These values must stay in step with `useCreateChatDialog`
 * (`pages/next-chats/hooks/use-create-chat.ts`). They are not arbitrary: an
 * empty `parameters` list leaves the `{knowledge}` and `{date}` placeholders in
 * the prompt unresolved, and flipping `refine_multiturn` on would enable
 * multi-turn refinement the user never asked for. A backfilled assistant should
 * behave like one created through the UI, not like a differently configured one.
 *
 * `system` is passed in because the caller resolves it through `t`; it falls
 * back to the same `chat.systemInitialValue` the create dialog uses.
 */
export function promptConfigDefaults(
  systemPrompt: string,
): Partial<PromptConfig> {
  return {
    quote: true,
    keyword: false,
    tts: false,
    system: systemPrompt,
    refine_multiturn: false,
    parameters: [
      { key: 'knowledge', optional: false },
      { key: 'date', optional: true },
    ],
    reasoning: false,
    // Optional in the schema, but `null` is not: defaulting it keeps a persisted
    // null from making the form unvalidatable.
    cross_languages: [],
  };
}

/**
 * Fills in the keys `useChatSettingSchema` requires, so a chat persisted with an
 * incomplete `prompt_config` becomes editable instead of failing validation on
 * every save.
 *
 * A `null` is treated as missing for keys that have a default — the API can
 * persist `keyword: null`, which the schema rejects just like a missing key.
 * Keys without a default keep their `null`: `agentic_rag: null` means "no
 * override", and rewriting it would change what the chat inherits. An
 * explicitly empty `system` is likewise kept, so validation points at the field
 * the user needs to fill rather than at a value we substituted.
 */
export function withPromptConfigDefaults(
  promptConfig: Partial<PromptConfig> | null | undefined,
  systemPrompt: string,
): Partial<PromptConfig> {
  const defaults = promptConfigDefaults(systemPrompt);
  const merged: Partial<PromptConfig> = { ...defaults };
  for (const [key, value] of Object.entries(promptConfig ?? {})) {
    const isDefaultedKey = Object.prototype.hasOwnProperty.call(defaults, key);
    // `undefined` never clobbers a default; for a defaulted key neither does
    // `null`. Spreading either one verbatim is how the backfill silently fails.
    if (value !== undefined && (value !== null || !isDefaultedKey)) {
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
