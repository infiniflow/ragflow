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

import { renderHook } from '@testing-library/react';

import { LLMFactory } from '@/constants/llm';

import { useProviderFields } from './use-provider-fields';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const renderFields = (urlHint?: string) =>
  renderHook(() =>
    useProviderFields({
      llmFactory: LLMFactory.Ollama,
      hideWhenInstanceExists: () => false,
      urlHint,
    }),
  ).result.current.fields;

const placeholderOf = (fields: unknown[], name: string) =>
  (fields as { name: string; placeholder?: string }[]).find(
    (field) => field.name === name,
  )?.placeholder;

describe('provider endpoint placeholder', () => {
  it('shows the catalog url_hint instead of the generic hint', () => {
    expect(
      placeholderOf(
        renderFields('http://host.docker.internal:11434'),
        'base_url',
      ),
    ).toBe('http://host.docker.internal:11434');
  });

  it('falls back to the i18n hint when the catalog has no url_hint', () => {
    expect(placeholderOf(renderFields(), 'base_url')).toBe(
      'baseUrlNameMessage',
    );
  });

  it('never leaks url_hint into other fields', () => {
    expect(
      placeholderOf(
        renderFields('http://host.docker.internal:11434'),
        'api_key',
      ),
    ).toBe('apiKeyMessage');
  });
});
