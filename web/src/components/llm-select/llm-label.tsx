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
import { useTranslation } from 'react-i18next';
import { LlmIcon } from '../svg-icon';

interface IProps {
  value?: string;
  ownerTenantId?: string;
  /** Mark the model as provided by the platform. */
  platform?: boolean;
  /** Name to show when the value itself is not readable (a model ID). */
  label?: string;
  /** Leave the full-name tooltip to a container that has its own. */
  hideTitle?: boolean;
}

/** Marks a model that the platform provides to every tenant. */
export function PlatformModelBadge() {
  const { t } = useTranslation();
  return (
    <span className="shrink-0 rounded-md border-0.5 border-border-button px-1.5 py-0.5 text-[10px] leading-none text-text-primary">
      {t('setting.platformProvided')}
    </span>
  );
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
    return { ...parsed, platform: false };
  }
  const model = value ? models.find((m) => m.model_id === value) : undefined;
  return model
    ? {
        model_name: model.name,
        model_instance: model.instance_name,
        model_provider: model.provider_name,
        platform: model.source === 'platform',
      }
    : null;
}

export const LLMLabel = ({
  value,
  ownerTenantId,
  platform,
  hideTitle,
}: IProps) => {
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
      {/* Name and instance truncate as one text, so the instance is cut
          first and the source badge stays visible in a narrow picker. */}
      <span
        className="font-medium truncate min-w-[3ch]"
        title={
          hideTitle
            ? undefined
            : [display.model_name, display.model_instance]
                .filter(Boolean)
                .join(' ')
        }
      >
        {display.model_name}
        {display.model_instance && ' '}
        {display.model_instance && (
          <span className="ml-0.5 font-normal text-text-secondary">
            {display.model_instance}
          </span>
        )}
      </span>
      {(platform || display.platform) && <PlatformModelBadge />}
    </div>
  );
};

/**
 * Warning marker for a persisted model value that resolves against no usable
 * model — mirrors the retrieval node's stale-dataset row. Best-effort name:
 * parsed composite name, else a model_id lookup in the (optionally
 * owner-scoped) added-model list, else the raw value.
 */
export const MissingModelLabel = ({ value, ownerTenantId, label }: IProps) => {
  const display = useModelDisplayName({ value, ownerTenantId });
  const name = display?.model_name ?? label ?? value;
  const fullName = [display?.model_name ?? name, display?.model_instance]
    .filter(Boolean)
    .join(' ');

  return (
    <span className="flex items-center gap-1.5 min-w-0 text-text-disabled">
      <TriangleAlert className="size-4 flex-shrink-0" />
      <span className="truncate" title={fullName}>
        {name}
      </span>
      {/* Tells it apart from a usable model of the same name. */}
      {display?.platform && <PlatformModelBadge />}
    </span>
  );
};

export default memo(LLMLabel);
