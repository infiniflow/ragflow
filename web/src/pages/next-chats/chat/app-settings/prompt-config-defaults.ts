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
 * Fields the schema declares as `.nullable()`, so `null` is a real value rather
 * than a missing one. `agentic_rag: null` means "no per-chat override"; every
 * other field rejects `null`.
 */
const NULLABLE_FIELDS = new Set<string>(['agentic_rag']);

/**
 * Drops `null` from fields the schema cannot accept, leaving `agentic_rag`
 * alone.
 *
 * The API persists `null` far more often than the types suggest. For a key with
 * a default the caller's spread restores the default afterwards; for one
 * without — `prologue`, `empty_response`, the per-provider API keys,
 * `web_search_provider` — omission is the only honest answer, because the
 * schema rejects `null` and materialising `''` would invent a value the user
 * never entered. A single surviving `null` makes the whole form unvalidatable,
 * so every one has to go.
 */
function withoutNulls(config: Partial<PromptConfig>): Partial<PromptConfig> {
  const result: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(config)) {
    if (value === undefined) {
      continue;
    }
    if (value === null && !NULLABLE_FIELDS.has(key)) {
      continue;
    }
    result[key] = value;
  }
  return result as Partial<PromptConfig>;
}

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
 * Normalises `parameters` rows so the schema accepts them.
 *
 * A row persisted without `optional` fails `z.boolean()`, and a row without a
 * usable `key` cannot satisfy `z.string().min(1)` — either one blocks the save.
 * Known rows inherit the flag the create dialog gives them; unknown rows default
 * to optional so a variable the user added is never silently made mandatory.
 */
function normalizeParameters(
  rows: PromptConfig['parameters'] | undefined,
  defaults: Partial<PromptConfig>,
): PromptConfig['parameters'] {
  if (!Array.isArray(rows)) {
    return defaults.parameters ?? [];
  }
  const knownOptional = new Map(
    (defaults.parameters ?? []).map(({ key, optional }) => [key, optional]),
  );
  return rows
    .filter(
      (row): row is { key: string; optional: boolean } =>
        !!row && typeof row.key === 'string' && row.key.trim() !== '',
    )
    .map((row) => ({
      ...row,
      optional:
        typeof row.optional === 'boolean'
          ? row.optional
          : (knownOptional.get(row.key) ?? true),
    }));
}

/**
 * Fills in the keys `useChatSettingSchema` requires, so a chat persisted with an
 * incomplete `prompt_config` becomes editable instead of failing validation on
 * every save.
 *
 * `undefined` and, for defaulted keys, `null` are both treated as missing — the
 * API can persist `keyword: null`, which the schema rejects just like an absent
 * key. `null` on non-defaulted keys is dropped rather than replaced, and an
 * explicitly empty `system` is kept, so validation points at the field the user
 * needs to fill rather than at a value we substituted.
 */
export function withPromptConfigDefaults(
  promptConfig: Partial<PromptConfig> | null | undefined,
  systemPrompt: string,
): Partial<PromptConfig> {
  const defaults = promptConfigDefaults(systemPrompt);
  const merged: Partial<PromptConfig> = {
    ...defaults,
    ...withoutNulls(promptConfig ?? {}),
  };

  merged.parameters = normalizeParameters(merged.parameters, defaults);

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
 *
 * Nulls are dropped here too: a persisted `prologue: null` the form never
 * touched would otherwise be sent straight back and rejected on the next load.
 */
export function mergePromptConfigForSubmit(
  persisted?: Partial<PromptConfig> | null,
  submitted?: Partial<PromptConfig> | null,
): Partial<PromptConfig> {
  return {
    ...withoutNulls(persisted ?? {}),
    ...withoutNulls(submitted ?? {}),
  };
}
