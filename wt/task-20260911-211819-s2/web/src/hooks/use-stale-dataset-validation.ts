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

import { get } from 'lodash';
import { useEffect, useMemo } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { useStaleDatasetIds } from './use-knowledge-request';

/**
 * Extend a form schema with staleness validation on its dataset-id field
 * (`fieldName`, a dotted path defaulting to `dataset_ids`). Only the
 * persisted ids are looked up: ids picked from the dataset select are valid
 * by construction, while a persisted id may reference a dataset that has
 * since been deleted or emptied of chunks — those stale ids are flagged here.
 */
export function useStaleDatasetFormSchema<T extends z.ZodTypeAny>(
  schema: T,
  persistedDatasetIds?: string[],
  fieldName = 'dataset_ids',
) {
  const { t } = useTranslation();
  const { staleDatasetIds, settled: datasetsFetched } =
    useStaleDatasetIds(persistedDatasetIds);

  const formSchema = useMemo(() => {
    const path = fieldName.split('.');
    return schema.superRefine((data: any, ctx) => {
      const ids = get(data, path) as string[] | undefined;
      if (ids?.some((id) => staleDatasetIds.has(id))) {
        ctx.addIssue({
          path,
          message: t('chat.datasetUnavailable'),
          code: z.ZodIssueCode.custom,
        });
      }
    });
  }, [schema, staleDatasetIds, fieldName, t]);

  return { formSchema, datasetsFetched };
}

/**
 * A persisted dataset-id value never fires onChange validation, so once the
 * lookup of those ids has settled, revalidate explicitly — it may reference
 * datasets that have since been deleted or emptied of chunks.
 */
export function useRevalidateStaleDatasetIds(
  form: UseFormReturn<any>,
  datasetsFetched: boolean,
  fieldName = 'dataset_ids',
) {
  const datasetIds = useWatch({ control: form.control, name: fieldName });
  const trigger = form.trigger;

  useEffect(() => {
    if (!datasetsFetched || !datasetIds?.length) {
      return;
    }

    trigger(fieldName);
  }, [trigger, datasetsFetched, datasetIds?.length, fieldName]);
}
