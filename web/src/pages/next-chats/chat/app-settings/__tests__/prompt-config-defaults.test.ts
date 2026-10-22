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

import {
  mergePromptConfigForSubmit,
  promptConfigDefaults,
  withPromptConfigDefaults,
} from '../prompt-config-defaults';

const SYSTEM_PROMPT = 'You are an intelligent assistant.';

// Mirrors the required keys of `promptConfigSchema` in use-chat-setting-schema.
const BACKFILLED_PROMPT_CONFIG_KEYS = [
  'quote',
  'keyword',
  'tts',
  'system',
  'refine_multiturn',
] as const;

describe('withPromptConfigDefaults', () => {
  it('fills in keys that a chat created through the API omits', () => {
    // Shape returned by POST /api/v1/chats with a minimal prompt_config.
    const apiCreated = {
      parameters: [{ key: 'knowledge', optional: false }],
      quote: true,
    };

    const merged = withPromptConfigDefaults(apiCreated, SYSTEM_PROMPT);

    for (const key of BACKFILLED_PROMPT_CONFIG_KEYS) {
      expect(merged).toHaveProperty(key);
    }
    expect(merged.keyword).toBe(false);
    expect(merged.tts).toBe(false);
    expect(merged.refine_multiturn).toBe(true);
  });

  it('does not overwrite values the chat already has', () => {
    const existing = {
      quote: false,
      keyword: true,
      tts: true,
      refine_multiturn: false,
      system: 'custom system prompt',
      parameters: [{ key: 'knowledge', optional: true }],
    };

    const merged = withPromptConfigDefaults(existing, SYSTEM_PROMPT);

    // Every persisted value wins over the default of the same key.
    expect(merged).toMatchObject(existing);
  });

  it('backfills a missing system prompt so the form can validate', () => {
    // `system` is `z.string().min(1)`, so leaving it undefined fails validation
    // just as an empty string does.
    expect(withPromptConfigDefaults({}, SYSTEM_PROMPT).system).toBe(
      SYSTEM_PROMPT,
    );
    expect(
      withPromptConfigDefaults({ system: undefined }, SYSTEM_PROMPT).system,
    ).toBe(SYSTEM_PROMPT);
  });

  it('keeps a persisted system prompt that is present but empty', () => {
    // The spread is a backfill, not a repair: an explicitly empty prompt is the
    // user's own state, and validation should point at it.
    expect(withPromptConfigDefaults({ system: '' }, SYSTEM_PROMPT).system).toBe(
      '',
    );
  });

  it('treats null as missing for keys that have a default', () => {
    // The API can persist `keyword: null`, which the schema rejects just like a
    // missing key, so keeping it verbatim would leave the form unvalidatable.
    const merged = withPromptConfigDefaults(
      { keyword: null, tts: null, parameters: null } as any,
      SYSTEM_PROMPT,
    );

    expect(merged.keyword).toBe(false);
    expect(merged.tts).toBe(false);
    expect(merged.parameters).toEqual([]);
  });

  it('keeps null for keys that have no default', () => {
    // `agentic_rag: null` means "no per-chat override" — rewriting it would
    // change what the assistant inherits.
    const merged = withPromptConfigDefaults(
      { agentic_rag: null } as any,
      SYSTEM_PROMPT,
    );

    expect(merged.agentic_rag).toBeNull();
  });

  it('handles a missing or null prompt_config', () => {
    for (const input of [undefined, null]) {
      const merged = withPromptConfigDefaults(input, SYSTEM_PROMPT);
      expect(merged).toEqual(promptConfigDefaults(SYSTEM_PROMPT));
      for (const key of BACKFILLED_PROMPT_CONFIG_KEYS) {
        expect(merged).toHaveProperty(key);
      }
    }
  });

  it('returns a new object rather than sharing the defaults', () => {
    const merged = withPromptConfigDefaults({}, SYSTEM_PROMPT);

    merged.keyword = true;

    expect(promptConfigDefaults(SYSTEM_PROMPT).keyword).toBe(false);
  });
});

describe('mergePromptConfigForSubmit', () => {
  it('keeps undeclared fields that zod stripped from the submitted values', () => {
    // `use_kg` and `toc_enhance` are part of PromptConfig but not of
    // promptConfigSchema, so the form values never carry them.
    const persisted = {
      use_kg: true,
      toc_enhance: true,
      keyword: false,
      system: 'persisted prompt',
    };
    const submitted = { keyword: true, system: 'edited prompt' };

    const merged = mergePromptConfigForSubmit(persisted, submitted);

    expect(merged).toMatchObject({
      use_kg: true,
      toc_enhance: true,
      keyword: true,
      system: 'edited prompt',
    });
  });

  it('handles a missing persisted or submitted config', () => {
    expect(mergePromptConfigForSubmit(undefined, { keyword: true })).toEqual({
      keyword: true,
    });
    expect(mergePromptConfigForSubmit({ use_kg: true }, null)).toEqual({
      use_kg: true,
    });
    expect(mergePromptConfigForSubmit(null, undefined)).toEqual({});
  });
});
