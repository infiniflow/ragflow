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

import { declaredContractFields } from './action-contract';

describe('declaredContractFields', () => {
  it('reads an empty declaration as all six', () => {
    expect(declaredContractFields('')).toHaveLength(6);
    expect(declaredContractFields(undefined)).toHaveLength(6);
  });

  it('reads a declared subset and drops unknown names', () => {
    expect(declaredContractFields('authorization|preconditions')).toEqual([
      'authorization',
      'preconditions',
    ]);
    // An unknown name is dropped rather than passed through: a typo must not
    // become a field nobody extracts.
    expect(declaredContractFields('authorization|auth')).toEqual([
      'authorization',
    ]);
  });

  it('falls back to all six when nothing recognisable is declared', () => {
    expect(declaredContractFields('typo')).toHaveLength(6);
  });

  it('always keeps authorization, as the server does', () => {
    // It decides whether the action may ever run unattended, and a field the
    // extractor was never asked for can never be stated by the document.
    expect(declaredContractFields('audit_required|follow_ups')).toEqual([
      'authorization',
      'audit_required',
      'follow_ups',
    ]);
  });

  it('collapses a repeated name into one declaration', () => {
    expect(
      declaredContractFields('authorization|authorization|follow_ups'),
    ).toEqual(['authorization', 'follow_ups']);
  });
});
