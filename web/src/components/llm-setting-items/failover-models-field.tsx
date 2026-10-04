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

import { LLMLabel, MissingModelLabel } from '@/components/llm-select/llm-label';
import { ModelTreeSelect } from '@/components/model-tree-select';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
} from '@/components/ui/form';
import { Button } from '@/components/ui/button';
import { useFetchAllAddedModels } from '@/hooks/use-llm-request';
import { ArrowDown, ArrowUp, X } from 'lucide-react';
import { useTranslation } from 'react-i18next';

export type FailoverModelsFieldProps = {
  name?: string;
  ownerTenantId?: string;
  /** The dialog's own model, rendered read-only as the chain's head. */
  primaryLlmId?: string;
};

type FailoverModelsProps = Omit<FailoverModelsFieldProps, 'name'> & {
  value?: string[];
  onChange?: (value: string[]) => void;
};

/**
 * FailoverModels edits the ordered list of fallback models a dialog may switch
 * to when the primary model hits a provider-level failure.
 *
 * Order is the author's priority: the backend hands this list to
 * NewFailoverEinoChatModel, which walks it in order and keeps a member that
 * served a call in front. The primary model is deliberately NOT part of this
 * list — it is the dialog's own model selection and always heads the chain, so
 * including it here would let the list contradict the primary.
 *
 * This is unrelated to the "Multiple models" panel button, which opens N
 * side-by-side conversations for a human to compare answers.
 */
function FailoverModels({
  ownerTenantId,
  primaryLlmId,
  value = [],
  onChange,
}: FailoverModelsProps) {
  const { t } = useTranslation();
  const { data: allModels, isFetched: modelsFetched } = useFetchAllAddedModels(
    undefined,
    ownerTenantId,
  );
  const members = Array.isArray(value) ? value : [];

  // A member that equals the primary would be attempted twice in a row; drop it
  // on the way in so a stale saved value cannot produce a pointless duplicate.
  const distinct = members.filter((id) => id && id !== primaryLlmId);

  const emit = (next: string[]) => {
    onChange?.(next.filter((id, index) => id && next.indexOf(id) === index));
  };

  const addMember = (id: string) => {
    if (!id || id === primaryLlmId || distinct.includes(id)) return;
    emit([...distinct, id]);
  };

  const removeAt = (index: number) =>
    emit(distinct.filter((_, i) => i !== index));

  const move = (index: number, delta: number) => {
    const target = index + delta;
    if (target < 0 || target >= distinct.length) return;
    const next = [...distinct];
    [next[index], next[target]] = [next[target], next[index]];
    emit(next);
  };

  /**
   * A member the backend can no longer resolve is still shown, marked stale
   * rather than dropped: the backend skips it with a warning, so removing the
   * row here would hide a real (if degraded) configuration from the author.
   */
  const isStale = (id: string) =>
    modelsFetched && !allModels?.some((m) => m.model_id === id);

  return (
    <FormItem>
      <FormLabel tooltip={t('chat.failoverModelsTip')}>
        {t('chat.failoverModels')}
      </FormLabel>
      <FormControl>
        <div className="space-y-2" data-testid="chat-failover-models">
          <div className="flex items-center gap-1.5 text-xs">
            <span className="text-text-secondary shrink-0">
              {t('chat.failoverModelsPrimaryLabel')}
            </span>
            {primaryLlmId ? (
              <div className="min-w-0 flex-1">
                <LLMLabel value={primaryLlmId} ownerTenantId={ownerTenantId} />
              </div>
            ) : (
              <span className="text-text-disabled truncate">
                {t('chat.failoverModelsNoPrimary')}
              </span>
            )}
          </div>

          {distinct.length === 0 ? (
            <p
              className="text-text-disabled text-xs"
              data-testid="chat-failover-empty"
            >
              {t('chat.failoverModelsEmpty')}
            </p>
          ) : (
            <ul className="space-y-1">
              {distinct.map((id, index) => (
                <li
                  key={id}
                  className="border-border-button flex items-center gap-2 rounded border px-2 py-1"
                  data-testid={`chat-failover-member-${index}`}
                >
                  <span className="text-text-secondary w-4 shrink-0 text-xs">
                    {index + 1}
                  </span>
                  <div className="min-w-0 flex-1 text-sm">
                    {isStale(id) ? (
                      <MissingModelLabel
                        value={id}
                        ownerTenantId={ownerTenantId}
                      />
                    ) : (
                      <LLMLabel value={id} ownerTenantId={ownerTenantId} />
                    )}
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    disabled={index === 0}
                    onClick={() => move(index, -1)}
                    data-testid={`chat-failover-up-${index}`}
                  >
                    <ArrowUp className="size-3.5" />
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    disabled={index === distinct.length - 1}
                    onClick={() => move(index, 1)}
                    data-testid={`chat-failover-down-${index}`}
                  >
                    <ArrowDown className="size-3.5" />
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    onClick={() => removeAt(index)}
                    data-testid={`chat-failover-remove-${index}`}
                  >
                    <X className="size-3.5" />
                  </Button>
                </li>
              ))}
            </ul>
          )}

          <div className="flex items-center gap-2">
            <ModelTreeSelect
              modelTypes={['chat']}
              ownerTenantId={ownerTenantId}
              allowClear
              placeholder={t('chat.failoverModelsAdd')}
              onChange={addMember}
              testId="chat-failover-add-select"
            />
          </div>
        </div>
      </FormControl>
    </FormItem>
  );
}

/**
 * FailoverModelsField binds FailoverModels to a react-hook-form field whose value
 * is the raw string[] stored in llm_setting.
 */
export function FailoverModelsField(props: FailoverModelsFieldProps) {
  return (
    <FormField
      name={props.name ?? 'failover_llm_ids'}
      render={({ field }) => (
        <FailoverModels
          {...props}
          value={Array.isArray(field.value) ? field.value : []}
          onChange={field.onChange}
        />
      )}
    />
  );
}
