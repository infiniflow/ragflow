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

import type { ReactNode } from 'react';
import type { InputSelectOption } from '../ui/input-select';

export type VariableOptionGroup = {
  title?: ReactNode;
  options?: { label?: ReactNode; value?: string }[];
};

// Query variable options arrive grouped by upstream node; flatten them into
// InputSelect options whose value is the `{...}` reference text the agent
// runtime resolves when the retrieval tool runs.
export function flattenVariableOptions(
  groups: VariableOptionGroup[],
): InputSelectOption[] {
  return groups.flatMap((group) => {
    const groupTitle = typeof group.title === 'string' ? group.title : '';
    return (group.options ?? [])
      .filter((leaf) => !!leaf.value)
      .map((leaf) => {
        const value = leaf.value as string;
        const leafLabel = typeof leaf.label === 'string' ? leaf.label : value;
        return {
          value: `{${value}}`,
          label: groupTitle ? `${groupTitle} / ${leafLabel}` : leafLabel,
        };
      });
  });
}
