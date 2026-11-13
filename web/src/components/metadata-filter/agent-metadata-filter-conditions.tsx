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

import { useBuildQueryVariableOptions } from '@/pages/agent/hooks/use-get-begin-query';
import { useMemo } from 'react';
import { MetadataFilterConditions } from './metadata-filter-conditions';
import { flattenVariableOptions } from './utils';

// Agent-only variant of the manual metadata conditions: besides the candidate
// values from the knowledge base summary, the value field also offers the
// upstream canvas variables as options. Kept out of the shared component so
// non-agent pages never mount the agent query hooks.
export function AgentMetadataFilterConditions({
  kbIds,
  prefix = '',
}: {
  kbIds: string[];
  prefix?: string;
}) {
  const variableOptionGroups = useBuildQueryVariableOptions();
  const variableOptions = useMemo(
    () => flattenVariableOptions(variableOptionGroups),
    [variableOptionGroups],
  );

  return (
    <MetadataFilterConditions
      kbIds={kbIds}
      prefix={prefix}
      variableOptions={variableOptions}
    />
  );
}
