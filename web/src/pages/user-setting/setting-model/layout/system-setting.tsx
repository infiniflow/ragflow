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

import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { MissingModelLabel } from '@/components/llm-select/llm-label';
import { ModelTreeSelect, ModelTypeMap } from '@/components/model-tree-select';
import { Button } from '@/components/ui/button';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { FieldToModelType, ModelTypeToField } from '@/constants/llm';
import { useTranslate } from '@/hooks/common-hooks';
import {
  useFetchDefaultModels,
  useSetDefaultModel,
} from '@/hooks/use-llm-request';
import { IDefaultModel } from '@/interfaces/database/llm';
import { ISetDefaultModelRequestBody } from '@/interfaces/request/llm';
import { buildModelValue, parseModelValue } from '@/utils/llm-util';
import { CircleQuestionMark, LucideLoader2, TriangleAlert } from 'lucide-react';
import {
  RefObject,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';

/** How the current default of one model type was chosen. */
type DefaultState =
  | 'user'
  | 'platform'
  | 'unavailable'
  | 'platformUnavailable'
  | 'unset';

const DefaultStateHintKeys: Record<DefaultState, string> = {
  user: 'defaultModelSourceUser',
  platform: 'defaultModelSourcePlatform',
  unavailable: 'defaultModelUnavailable',
  platformUnavailable: 'defaultModelPlatformUnavailable',
  unset: 'defaultModelUnset',
};

interface ModelFieldItemProps {
  id: string;
  label: string;
  value: string;
  state: DefaultState;
  /** Readable name of an unavailable saved model. */
  missingLabel?: string;
  tooltip?: string;
  isRequired?: boolean;
  /** True while a change of this row is being saved. */
  saving: boolean;
  /** False until the saved defaults have loaded. */
  loaded: boolean;
  onChange: (id: string, value: string) => void;
  onReset: (id: string) => Promise<unknown>;
}

/**
 * Moves focus to `target` once it is lost because the focused element went
 * away (it then falls to the body). Focus the user put elsewhere, or that a
 * closing dialog hands back to a control that still exists, stays put.
 */
function restoreFocusWhenLost(target: RefObject<HTMLElement>, tries = 20) {
  window.setTimeout(() => {
    const active = document.activeElement;
    // A closing dialog still holds focus; wait until it hands it back.
    if (active?.closest('[role="alertdialog"]') && tries > 0) {
      restoreFocusWhenLost(target, tries - 1);
      return;
    }
    if (!active || active === document.body || !active.isConnected) {
      target.current?.focus();
    }
  }, 50);
}

function ModelFieldItem({
  label,
  value,
  state,
  missingLabel,
  tooltip,
  id,
  isRequired,
  saving,
  loaded,
  onChange,
  onReset,
}: ModelFieldItemProps) {
  const { t } = useTranslate('setting');
  const pickerRef = useRef<HTMLButtonElement>(null);
  // Changes made while a save is running are queued, not dropped.
  const handleChange = useCallback(
    (val: string) => onChange(id, val),
    [id, onChange],
  );
  const handleReset = useCallback(async () => {
    try {
      await onReset(id);
    } catch {
      // The request layer already reports the failure.
    } finally {
      // The reset button may disappear with the user's choice.
      restoreFocusWhenLost(pickerRef);
    }
  }, [id, onReset]);
  // The picker still lists a disabled tenant model, so an unavailable
  // choice is marked here the same way as a deleted one.
  const unavailable =
    state === 'unavailable' || state === 'platformUnavailable';
  // A platform default whose model record is gone has no readable name;
  // its ID is not one either.
  const displayLabel =
    missingLabel ??
    (state === 'platformUnavailable'
      ? t('defaultModelPlatformMissingName')
      : undefined);
  const renderUnavailable = useCallback(
    () => <MissingModelLabel value={value} label={displayLabel} />,
    [value, displayLabel],
  );
  const labelId = `default-model-${id}-label`;
  const hintId = `default-model-${id}-hint`;
  // A required model without a default leaves new chats and datasets
  // without a model, so it is shown as a warning.
  const warn =
    loaded &&
    (state === 'unavailable' ||
      state === 'platformUnavailable' ||
      (isRequired && state === 'unset'));
  const hintKey =
    isRequired && state === 'unset'
      ? 'defaultModelRequiredUnset'
      : DefaultStateHintKeys[state];

  return (
    <div
      role="group"
      aria-labelledby={labelId}
      aria-describedby={hintId}
      aria-busy={saving || !loaded}
      className="flex flex-col gap-1.5 @md:flex-row @md:gap-3 @md:items-start"
    >
      <label
        id={labelId}
        className="block text-sm font-normal text-text-secondary @md:pt-2 @md:w-1/4 @md:max-w-[150px]"
      >
        {isRequired && (
          <span className="text-state-error" aria-hidden>
            *
          </span>
        )}
        {label}
        {/* The asterisk is hidden from screen readers; say it in words. */}
        {isRequired && (
          <span className="sr-only">{` (${t('defaultModelRequired')})`}</span>
        )}
        {tooltip && (
          <Tooltip>
            <TooltipContent>{tooltip}</TooltipContent>
            <TooltipTrigger>
              <CircleQuestionMark
                size={12}
                className="ml-1 text-text-secondary text-xs"
              />
            </TooltipTrigger>
          </Tooltip>
        )}
      </label>
      <div className="w-full flex-1 min-w-0 space-y-1.5 @md:w-3/4">
        <ModelTreeSelect
          modelTypes={ModelTypeMap[id as keyof typeof ModelTypeMap] ?? ['chat']}
          value={value}
          onChange={handleChange}
          // Until the saved default is known, a choice could overwrite it
          // unseen.
          disabled={!loaded}
          placeholder={
            loaded ? t('selectModelPlaceholder') : t('defaultModelsLoading')
          }
          showSearch
          missingValueLabel={displayLabel}
          renderSelected={unavailable ? renderUnavailable : undefined}
          ref={pickerRef}
        />
        {/* Reset sits with the hint, so the picker keeps the full width. */}
        <div className="flex items-start justify-between gap-2 min-w-0">
          <p
            id={hintId}
            className={
              warn && !saving
                ? 'flex min-w-0 break-words items-center gap-1 min-h-6 text-xs text-text-primary'
                : 'flex min-w-0 break-words items-center gap-1 min-h-6 text-xs text-text-secondary'
            }
          >
            {saving ? (
              <>
                <LucideLoader2 className="size-3.5 shrink-0 animate-spin" />
                {t('defaultModelSaving')}
              </>
            ) : (
              <>
                {/* The icon carries the warning color; small text in it is
                    too faint on a light background. */}
                {warn && (
                  <TriangleAlert className="size-3.5 shrink-0 text-state-warning" />
                )}
                {loaded ? t(hintKey) : null}
              </>
            )}
          </p>
          {(state === 'user' || state === 'unavailable') && (
            <ConfirmDeleteDialog
              title={t('resetDefaultModelTitle')}
              okButtonText={t('resetDefaultModel')}
              onOk={handleReset}
              content={{
                title: label,
                node: (
                  <span className="text-sm text-text-secondary">
                    {t(
                      isRequired
                        ? 'resetRequiredDefaultModelDescription'
                        : id === 'embd_id'
                          ? 'resetEmbeddingDefaultModelDescription'
                          : 'resetDefaultModelDescription',
                    )}
                  </span>
                ),
              }}
            >
              <Button
                variant="ghost"
                size="xs"
                className="shrink-0 px-2 text-xs"
                aria-label={t('resetDefaultModelFor', { type: label })}
              >
                {t('resetDefaultModel')}
              </Button>
            </ConfirmDeleteDialog>
          )}
        </div>
      </div>
    </div>
  );
}

/**
 * Reads one default from the server: its saved reference, kept even when the
 * model is no longer usable so that it can be shown and replaced.
 */
function describeDefault(model: IDefaultModel | undefined): {
  value: string;
  state: DefaultState;
  missingLabel?: string;
} {
  if (!model) return { value: '', state: 'unset' };
  // Keep the ID as the value so an old name never maps to another model with
  // the same name; the saved name is only used for display.
  const value =
    model.model_id ||
    model.model_ref ||
    (model.model_name && model.model_instance && model.model_provider
      ? buildModelValue({
          model_name: model.model_name,
          model_instance: model.model_instance,
          model_provider: model.model_provider,
        })
      : '');
  const missingLabel =
    parseModelValue(model.model_ref ?? '')?.model_name ||
    model.model_ref ||
    model.model_name ||
    undefined;
  if (!model.enable) {
    // Resetting cannot fix an unavailable platform default: the user has no
    // choice of their own, so only choosing a model helps.
    return {
      value,
      missingLabel,
      state:
        model.source === 'platform' ? 'platformUnavailable' : 'unavailable',
    };
  }
  return { value, state: model.source === 'platform' ? 'platform' : 'user' };
}

function SystemSetting() {
  const { t } = useTranslate('setting');
  const {
    data: defaultModels,
    isFetched: loaded,
    isError: loadFailed,
    loading: reloading,
    refetch,
    everLoaded,
  } = useFetchDefaultModels();
  const { setDefaultModel } = useSetDefaultModel();
  // Rows save independently; changes of one row are queued in order.
  const [savingFields, setSavingFields] = useState<ReadonlySet<string>>(
    () => new Set(),
  );

  // Saves of one row run in order, so the last change wins; rows are
  // independent.
  const pendingSaves = useRef<Record<string, Promise<unknown>>>({});
  const saveField = useCallback(
    (field: string, params: ISetDefaultModelRequestBody) => {
      setSavingFields((current) => new Set(current).add(field));
      const previous = pendingSaves.current[field] ?? Promise.resolve();
      const save = previous
        .catch(() => undefined)
        .then(() => setDefaultModel(params));
      pendingSaves.current[field] = save;
      return save.finally(() => {
        // A later change of this row is still running.
        if (pendingSaves.current[field] !== save) return;
        delete pendingSaves.current[field];
        setSavingFields((current) => {
          const next = new Set(current);
          next.delete(field);
          return next;
        });
      });
    },
    [setDefaultModel],
  );

  const handleFieldChange = useCallback(
    async (field: string, value: string) => {
      const modelType = FieldToModelType[field];
      if (!modelType || !value) return;
      const parsed = parseModelValue(value);
      try {
        await saveField(
          field,
          parsed
            ? { ...parsed, model_type: modelType }
            : { model_id: value, model_type: modelType },
        );
      } catch {
        // The request layer reports the error; keep the saved selection.
      }
    },
    [saveField],
  );

  // Clearing the user's choice lets the platform default, if any, apply.
  const handleReset = useCallback(
    (field: string) =>
      saveField(field, { model_id: '', model_type: FieldToModelType[field] }),
    [saveField],
  );
  const headingRef = useRef<HTMLHeadingElement>(null);
  // The alert and its button go away on success; keep focus on the page.
  const handleRetry = useCallback(async () => {
    if (reloading) return;
    const result = await refetch();
    if (!result.isError) restoreFocusWhenLost(headingRef);
  }, [refetch, reloading]);
  // A failed read is not retried when other pages mount; this page shows the
  // failure and offers Retry, so it asks once more when it opens.
  const refetchedOnOpen = useRef(false);
  useEffect(() => {
    if (refetchedOnOpen.current) return;
    refetchedOnOpen.current = true;
    if (loadFailed) refetch();
  }, [loadFailed, refetch]);

  const defaults = useMemo(() => {
    const byField: Record<string, ReturnType<typeof describeDefault>> = {};
    Object.entries(ModelTypeToField).forEach(([modelType, field]) => {
      byField[field] = describeDefault(
        defaultModels.find((model) => model.model_type === modelType),
      );
    });
    return byField;
  }, [defaultModels]);

  const llmList = useMemo(() => {
    return [
      {
        id: 'llm_id',
        label: t('chatModel'),
        isRequired: true,
        tooltip: t('chatModelTip'),
      },
      {
        id: 'embd_id',
        label: t('embeddingModel'),
        tooltip: t('embeddingModelTip'),
      },
      {
        id: 'img2txt_id',
        label: t('img2txtModel'),
        tooltip: t('img2txtModelTip'),
      },
      {
        id: 'asr_id',
        label: t('sequence2txtModel'),
        tooltip: t('sequence2txtModelTip'),
      },
      {
        id: 'rerank_id',
        label: t('rerankModel'),
        tooltip: t('rerankModelTip'),
      },
      {
        id: 'tts_id',
        label: t('ttsModel'),
        tooltip: t('ttsModelTip'),
      },
      {
        // The decision model judges whether a retrieved passage is worth
        // putting in front of the chat model. It sits next to the chat model
        // because it is chosen the same way and independently of it.
        id: 'decision_id',
        label: t('decisionModel'),
        tooltip: t('decisionModelTip'),
      },
    ].map((item) => ({
      ...item,
      ...(defaults[item.id] ?? { value: '', state: 'unset' as const }),
    }));
  }, [defaults, t]);

  return (
    <article className="rounded-lg w-full">
      <header className="py-5 px-10">
        <h2
          ref={headingRef}
          tabIndex={-1}
          className="text-2xl font-medium text-text-primary outline-none"
        >
          {t('systemModelSettings')}
        </h2>
        <p className="mt-1 text-sm text-text-secondary ">
          {t('systemModelDescription')}
        </p>
      </header>

      {loadFailed && (
        <div
          role="alert"
          className="flex flex-wrap items-center gap-3 px-10 pt-6 text-sm text-state-error"
        >
          <TriangleAlert className="size-4 shrink-0" />
          <span>
            {t(
              everLoaded
                ? 'defaultModelsReloadFailed'
                : 'defaultModelsLoadFailed',
            )}
          </span>
          {/* aria-disabled rather than disabled keeps focus on the button
              while it reloads, so a failed retry can be repeated. */}
          <Button
            variant="outline"
            className="h-8"
            aria-disabled={reloading}
            onClick={handleRetry}
          >
            {reloading && <LucideLoader2 className="animate-spin" />}
            {t('defaultModelsRetry')}
          </Button>
        </div>
      )}
      {/* Before the first load, rows would present every default as not
          set; after it, a failed refresh keeps them (and focus) in place. */}
      {(!loadFailed || everLoaded) && (
        // A narrow page stacks each label above its picker, so the picker
        // keeps room for the model name and its source badge.
        <div className="@container px-10 py-6 space-y-6 max-h-[70vh] overflow-y-auto ">
          {llmList.map((item) => (
            <ModelFieldItem
              key={item.id}
              {...item}
              saving={savingFields.has(item.id)}
              loaded={loaded}
              onChange={handleFieldChange}
              onReset={handleReset}
            />
          ))}
        </div>
      )}
    </article>
  );
}

export default SystemSetting;
