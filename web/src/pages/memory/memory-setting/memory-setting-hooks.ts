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
 * The model selects become read-only once messages exist, so a stale
 * persisted model can only be fixed while the memory is still empty —
 * blocking validation applies only then; afterwards the select's warning
 * marker is the only signal.
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
          if (!modelsFetched || !modelsEditable) return;
          if (values.llm_id && !validLlmIds.has(values.llm_id)) {
            ctx.addIssue({
              path: ['llm_id'],
              message: t('common.modelUnavailable'),
              code: z.ZodIssueCode.custom,
            });
          }
          if (values.embd_id && !validEmbdIds.has(values.embd_id)) {
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
 * and the error should be visible before submit. When the selects turn
 * read-only (messages exist), drop any stale error instead.
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
    if (!modelsEditable) {
      clearErrors(['llm_id', 'embd_id']);
      return;
    }
    if (llmId) trigger('llm_id');
    if (embdId) trigger('embd_id');
  }, [trigger, clearErrors, modelsFetched, modelsEditable, llmId, embdId]);
};
