import { useStaleDatasetIds } from '@/hooks/use-knowledge-request';
import { useEffect, useMemo } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';

/**
 * Extend a form schema containing `dataset_ids` with staleness validation.
 * Only the persisted ids are looked up: ids picked from the dataset select
 * are valid by construction, while a persisted id may reference a dataset
 * that has since been deleted or emptied of chunks — those stale ids are
 * flagged here.
 */
export function useStaleDatasetFormSchema<T extends z.ZodTypeAny>(
  schema: T,
  persistedDatasetIds?: string[],
) {
  const { t } = useTranslation();
  const { staleDatasetIds, settled: datasetsFetched } =
    useStaleDatasetIds(persistedDatasetIds);

  const formSchema = useMemo(
    () =>
      schema.superRefine((data: any, ctx) => {
        if (data.dataset_ids?.some((id: string) => staleDatasetIds.has(id))) {
          ctx.addIssue({
            path: ['dataset_ids'],
            message: t('chat.datasetUnavailable'),
            code: z.ZodIssueCode.custom,
          });
        }
      }),
    [schema, staleDatasetIds, t],
  );

  return { formSchema, datasetsFetched };
}

/**
 * A persisted dataset_ids value never fires onChange validation, so once the
 * lookup of those ids has settled, revalidate explicitly — it may reference
 * datasets that have since been deleted or emptied of chunks.
 */
export function useRevalidateStaleDatasetIds(
  form: UseFormReturn<any>,
  datasetsFetched: boolean,
) {
  const datasetIds = useWatch({ control: form.control, name: 'dataset_ids' });
  const trigger = form.trigger;

  useEffect(() => {
    if (!datasetsFetched || !datasetIds?.length) {
      return;
    }

    trigger('dataset_ids');
  }, [trigger, datasetsFetched, datasetIds?.length]);
}
