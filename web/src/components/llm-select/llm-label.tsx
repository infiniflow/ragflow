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

import { useFetchAllAddedModels } from '@/hooks/use-llm-request';
import { parseModelValue } from '@/utils/llm-util';
import { TriangleAlert } from 'lucide-react';
import { memo } from 'react';
import { LlmIcon } from '../svg-icon';

interface IProps {
  value?: string;
  ownerTenantId?: string;
}

/**
 * Resolve the display name for a persisted model value without judging its
 * availability: composite values yield their parsed name, plain model_id
 * values are looked up in the (optionally owner-scoped) added-model list.
 */
function useModelDisplayName({ value, ownerTenantId }: IProps) {
  const { data: models } = useFetchAllAddedModels(undefined, ownerTenantId);

  const parsed = value ? parseModelValue(value) : null;
  if (parsed?.model_name) {
    return parsed;
  }
  const model = value ? models.find((m) => m.model_id === value) : undefined;
  return model
    ? {
        model_name: model.name,
        model_instance: model.instance_name,
        model_provider: model.provider_name,
      }
    : null;
}

export const LLMLabel = ({ value, ownerTenantId }: IProps) => {
  const display = useModelDisplayName({ value, ownerTenantId });

  if (!display?.model_name) return null;

  return (
    <div className="flex items-center gap-1.5 min-w-0">
      <LlmIcon
        name={display.model_provider}
        width={22}
        height={22}
        imgClass="size-[22px] flex-shrink-0"
      />
      <span className="font-medium truncate">{display.model_name}</span>
      {display.model_instance && (
        <span className="text-text-secondary truncate flex-shrink-0">
          {display.model_instance}
        </span>
      )}
    </div>
  );
};

/**
 * Warning marker for a persisted model value that resolves against no usable
 * model — mirrors the retrieval node's stale-dataset row. Best-effort name:
 * parsed composite name, else a model_id lookup in the (optionally
 * owner-scoped) added-model list, else the raw value.
 */
export const MissingModelLabel = ({ value, ownerTenantId }: IProps) => {
  const display = useModelDisplayName({ value, ownerTenantId });

  return (
    <span className="flex items-center gap-1.5 text-text-disabled">
      <TriangleAlert className="size-4 flex-shrink-0" />
      <span className="truncate">{display?.model_name ?? value}</span>
    </span>
  );
};

export default memo(LLMLabel);
