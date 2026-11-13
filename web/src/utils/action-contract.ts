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
  ACTION_CONTRACT_FIELD_ORDER,
  ActionContractFieldName,
} from '@/interfaces/database/compilation-model';

/**
 * declaredContractFields reads a class's `|`-separated declaration.
 *
 * Mirrors the server's `contract.NormalizeDeclared`: unknown names are dropped,
 * duplicates collapse, nothing recognisable means all six, and authorization is
 * always included. Keeping the two in step is the point — the panel must not show
 * a box unticked for a field the extractor is still asked for.
 */
export function declaredContractFields(
  declaration: unknown,
): ActionContractFieldName[] {
  const text = String(declaration ?? '').trim();
  if (text === '') {
    // No declaration means "all six": a class that says nothing about its
    // contract still needs the contract, and the alternative reading — "this
    // action needs no fields" — is the dangerous one.
    return [...ACTION_CONTRACT_FIELD_ORDER];
  }
  const declared = [
    ...new Set(
      text
        .split('|')
        .map((item) => item.trim())
        .filter((item): item is ActionContractFieldName =>
          (ACTION_CONTRACT_FIELD_ORDER as string[]).includes(item),
        ),
    ),
  ];
  if (declared.length === 0) {
    return [...ACTION_CONTRACT_FIELD_ORDER];
  }
  // Authorization decides whether the action may run at all, so it cannot be
  // narrowed away: a field nobody was asked for can never be stated.
  return declared.includes('authorization')
    ? declared
    : ['authorization', ...declared];
}
