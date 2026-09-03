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

import { partialMatchKey } from '@tanstack/query-core';
import { DocumentKeys } from '../document-query-keys';

describe('DocumentKeys', () => {
  it('matches parameterized document lists with the list prefix', () => {
    const actual = DocumentKeys.list(
      'keyword',
      { current: 1, pageSize: 30 },
      { run: ['1'] },
    );

    expect(partialMatchKey(actual, DocumentKeys.all())).toBe(true);
  });

  it('matches parameterized document filters with the filter prefix', () => {
    const actual = DocumentKeys.filter('keyword', 'dataset-id');

    expect(partialMatchKey(actual, DocumentKeys.allFilters())).toBe(true);
  });
});
