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

import { render } from '@testing-library/react';
import { SearchInput } from '@/components/ui/input';
import { ChunkSearchEntry } from './chunk-search-entry';

// Emulates i18next's keyPrefix resolution so `t('search', {keyPrefix:'common'})`
// and `t('common.search')` both resolve to the same key.
jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: { keyPrefix?: string }) =>
      options?.keyPrefix ? `${options.keyPrefix}.${key}` : key,
  }),
}));

/**
 * The search box is a shared slot: the enterprise build fills it with the box
 * that also says what it reads, and open source must keep rendering exactly
 * what the bar used to inline. The expected markup below is that literal call,
 * so any drift in the stub — a placeholder, a width, a wrapper — fails here
 * instead of silently changing the open-source page.
 */
it('renders the box the shared bar used to inline', () => {
  const before = render(
    <SearchInput
      className="w-28"
      placeholder="common.search"
      onChange={() => {}}
      value="ragflow"
    />,
  );
  const after = render(
    <ChunkSearchEntry searchString="ragflow" handleInputChange={() => {}} />,
  );

  expect(after.container.innerHTML).toBe(before.container.innerHTML);
});
