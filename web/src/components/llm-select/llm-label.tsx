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
import { memo } from 'react';
import { LlmIcon } from '../svg-icon';

interface IProps {
  value?: string;
  ownerTenantId?: string;
}

export const LLMLabel = ({ value, ownerTenantId }: IProps) => {
  const { data: models } = useFetchAllAddedModels(undefined, ownerTenantId);

  const parsed = value ? parseModelValue(value) : null;

  let modelName = parsed?.model_name;
  let instanceName = parsed?.model_instance;
  let iconName = parsed ? parsed.model_provider : '';

  if (!modelName && value) {
    const model = models.find((m) => m.model_id === value);
    if (model) {
      modelName = model.name;
      instanceName = model.instance_name;
      iconName = model.provider_name;
    }
  }

  if (!modelName) return null;

  return (
    <div className="flex items-center gap-1.5 min-w-0">
      <LlmIcon
        name={iconName}
        width={22}
        height={22}
        imgClass="size-[22px] flex-shrink-0"
      />
      <span className="font-medium truncate">{modelName}</span>
      {instanceName && (
        <span className="text-text-secondary truncate flex-shrink-0">
          {instanceName}
        </span>
      )}
    </div>
  );
};

export default memo(LLMLabel);
