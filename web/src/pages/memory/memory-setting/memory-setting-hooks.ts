import { ModelTypeMap } from '@/components/model-tree-select';
import { useModelValidIds } from '@/hooks/use-llm-request';
import { IMemory } from '@/pages/memories/interface';
import { useEffect, useMemo } from 'react';
import {
  Control,
  UseFormClearErrors,
  UseFormTrigger,
  useWatch,
} from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { useFetchMemoryMessageList } from '../memory-message/hook';
import { advancedSettingsFormSchema } from './advanced-settings-form';
import { basicInfoSchema } from './basic-form';
import { memoryModelFormSchema } from './memory-model-form';

/**
 * Extends the static schema with an existence check against the added-model
 * list: a persisted value may reference a model that has since been deleted.
 * llm_id is checked in every state — a stale chat model breaks the memory's
 * chat flow, so the error surfaces (and blocks submit) even once messages
 * exist and the select has turned read-only. embd_id is checked only while
 * the memory is still empty: afterwards its select is read-only and the
 * stale value is tolerated, so the select's warning marker is the only
 * signal there.
 */
export const useMemoryFormSchema = (tenantId?: string) => {
  const { t } = useTranslation();
  const { data: messageData } = useFetchMemoryMessageList();
  const modelsEditable = !messageData?.messages?.total_count;
  const { validIds: validLlmIds, isFetched: modelsFetched } = useModelValidIds(
    ModelTypeMap.llm_id,
    tenantId,
  );
  const { validIds: validEmbdIds } = useModelValidIds(
    ModelTypeMap.embd_id,
    tenantId,
  );

  const formSchema = useMemo(
    () =>
      z
        .object({
          id: z.string(),
          ...basicInfoSchema,
          ...memoryModelFormSchema(t),
          ...advancedSettingsFormSchema,
        })
        .superRefine((values, ctx) => {
          if (!modelsFetched) return;
          if (values.llm_id && !validLlmIds.has(values.llm_id)) {
            ctx.addIssue({
              path: ['llm_id'],
              message: t('common.modelUnavailable'),
              code: z.ZodIssueCode.custom,
            });
          }
          if (
            modelsEditable &&
            values.embd_id &&
            !validEmbdIds.has(values.embd_id)
          ) {
            ctx.addIssue({
              path: ['embd_id'],
              message: t('common.modelUnavailable'),
              code: z.ZodIssueCode.custom,
            });
          }
        }),
    [t, modelsFetched, modelsEditable, validLlmIds, validEmbdIds],
  );

  return { formSchema, modelsFetched, modelsEditable };
};

/**
 * A persisted model value never fires onChange validation, so revalidate
 * once the model list has loaded — it may reference a since-deleted model,
 * and the error should be visible before submit. llm_id revalidates in
 * every state; embd_id only while the memory is still empty (read-only
 * afterwards), where a stale error is dropped instead.
 */
export const useRevalidatePersistedModels = ({
  control,
  trigger,
  clearErrors,
  modelsFetched,
  modelsEditable,
}: {
  control: Control<IMemory>;
  trigger: UseFormTrigger<IMemory>;
  clearErrors: UseFormClearErrors<IMemory>;
  modelsFetched: boolean;
  modelsEditable: boolean;
}) => {
  const llmId = useWatch({ control, name: 'llm_id' });
  const embdId = useWatch({ control, name: 'embd_id' });

  useEffect(() => {
    if (!modelsFetched) return;
    if (llmId) trigger('llm_id');
    if (modelsEditable) {
      if (embdId) trigger('embd_id');
    } else {
      clearErrors('embd_id');
    }
  }, [trigger, clearErrors, modelsFetched, modelsEditable, llmId, embdId]);
};
