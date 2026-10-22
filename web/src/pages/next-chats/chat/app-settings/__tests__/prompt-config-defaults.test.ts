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
  promptConfigDefaults,
  withPromptConfigDefaults,
} from '../prompt-config-defaults';

// Mirrors the required keys of `promptConfigSchema` in use-chat-setting-schema,
// minus `system` — the schema demands a non-empty value there and inventing a
// prompt is worse than letting validation point at the empty field.
const BACKFILLED_PROMPT_CONFIG_KEYS = [
  'quote',
  'keyword',
  'tts',
  'refine_multiturn',
] as const;

describe('withPromptConfigDefaults', () => {
  it('fills in keys that a chat created through the API omits', () => {
    // Shape returned by POST /api/v1/chats with a minimal prompt_config.
    const apiCreated = {
      parameters: [{ key: 'knowledge', optional: false }],
      quote: true,
    };

    const merged = withPromptConfigDefaults(apiCreated);

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

    const merged = withPromptConfigDefaults(existing);

    // Every persisted value wins over the default of the same key.
    expect(merged).toMatchObject(existing);
  });

  it('leaves `system` alone so an empty prompt still fails validation', () => {
    expect(withPromptConfigDefaults({})).not.toHaveProperty('system');
    expect(withPromptConfigDefaults({ system: '' }).system).toBe('');
  });

  it('handles a missing or null prompt_config', () => {
    for (const input of [undefined, null]) {
      const merged = withPromptConfigDefaults(input as any);
      expect(merged).toEqual(promptConfigDefaults);
      for (const key of BACKFILLED_PROMPT_CONFIG_KEYS) {
        expect(merged).toHaveProperty(key);
      }
    }
  });

  it('returns a new object rather than sharing the defaults', () => {
    const merged = withPromptConfigDefaults();

    merged.keyword = true;

    expect(promptConfigDefaults.keyword).toBe(false);
  });
});
