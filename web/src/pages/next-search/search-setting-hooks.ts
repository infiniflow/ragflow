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

// src/pages/next-search/search-setting-hooks.ts

import {
  LLMIdFormField,
  LlmSettingEnabledSchema,
  LlmSettingFieldSchema,
} from '@/components/llm-setting-items/next';
import { MetadataFilterSchema } from '@/components/metadata-filter';
import { ModelTypeMap } from '@/components/model-tree-select';
import { rerankCandidatesCountSchema } from '@/components/rerank-candidates-count-item';
import { useModelValidIds } from '@/hooks/use-llm-request';
import { useEffect, useMemo } from 'react';
import { Control, UseFormTrigger, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';

export const SearchSettingFormSchema = z
  .object({
    search_id: z.string().optional(),
    name: z.string().min(1, 'Name is required'),
    avatar: z.string().optional(),
    description: z.string().optional(),
    search_config: z.object({
      kb_ids: z.array(z.string()).min(1, 'At least one dataset is required'),
      vector_similarity_weight: z.number().min(0).max(1),
      web_search: z.boolean(),
      similarity_threshold: z.number(),
      use_kg: z.boolean(),
      rerank_id: z.string(),
      use_rerank: z.boolean(),
      ...rerankCandidatesCountSchema,
      summary: z.boolean(),
      llm_setting: z.object({ ...LlmSettingFieldSchema, ...LLMIdFormField }),
      related_search: z.boolean(),
      query_mindmap: z.boolean(),
      doc_ids: z.array(z.string()),
      chat_id: z.string(),
      highlight: z.boolean(),
      keyword: z.boolean(),
      chat_settingcross_languages: z.array(z.string()),
      reference_metadata: z
        .object({
          include: z.boolean().optional(),
          fields: z.array(z.string()).optional(),
        })
        .optional(),
      ...MetadataFilterSchema,
    }),
    ...LlmSettingEnabledSchema,
  })
  .superRefine((data, ctx) => {
    if (data.search_config.use_rerank && !data.search_config.rerank_id) {
      ctx.addIssue({
        path: ['search_config', 'rerank_id'],
        message: 'Rerank model is required when rerank is enabled',
        code: z.ZodIssueCode.custom,
      });
    }

    if (data.search_config.summary && !data.search_config.llm_setting?.llm_id) {
      ctx.addIssue({
        path: ['search_config', 'llm_setting', 'llm_id'],
        message: 'Model is required when AI Summary is enabled',
        code: z.ZodIssueCode.custom,
      });
    }
  });

export type SearchSettingFormData = z.infer<typeof SearchSettingFormSchema>;

/**
 * Extends the static schema with an existence check against the added-model
 * list: a persisted value may reference a model that has since been deleted.
 * Gated on the same switches as the required checks — a value hidden behind
 * a disabled switch is stripped on submit and must not block it.
 */
export const useSearchSettingFormSchema = () => {
  const { t } = useTranslation();
  const { validIds: validRerankIds, isFetched: modelsFetched } =
    useModelValidIds(ModelTypeMap.rerank_id);
  const { validIds: validLlmIds } = useModelValidIds(ModelTypeMap.llm_id);

  const formSchema = useMemo(
    () =>
      SearchSettingFormSchema.superRefine((formData, ctx) => {
        if (!modelsFetched) return;
        const { use_rerank, rerank_id, summary, llm_setting } =
          formData.search_config;
        if (use_rerank && rerank_id && !validRerankIds.has(rerank_id)) {
          ctx.addIssue({
            path: ['search_config', 'rerank_id'],
            message: t('common.modelUnavailable'),
            code: z.ZodIssueCode.custom,
          });
        }
        if (
          summary &&
          llm_setting?.llm_id &&
          !validLlmIds.has(llm_setting.llm_id)
        ) {
          ctx.addIssue({
            path: ['search_config', 'llm_setting', 'llm_id'],
            message: t('common.modelUnavailable'),
            code: z.ZodIssueCode.custom,
          });
        }
      }),
    [modelsFetched, validRerankIds, validLlmIds, t],
  );

  return { formSchema, modelsFetched };
};

/**
 * A persisted model value never fires onChange validation, so once the
 * model list has loaded, revalidate explicitly — it may reference a model
 * that has since been deleted, and the error should be visible before
 * submit. Gated on the switches for the same reason as the schema.
 *
 * The watched switch states are returned so the component can reuse them
 * for conditional rendering without subscribing to the same fields twice.
 */
export const useRevalidatePersistedModels = ({
  control,
  trigger,
  modelsFetched,
}: {
  control: Control<SearchSettingFormData>;
  trigger: UseFormTrigger<SearchSettingFormData>;
  modelsFetched: boolean;
}) => {
  const rerankModelEnabled = useWatch({
    control,
    name: 'search_config.use_rerank',
  });
  const aiSummaryEnabled = useWatch({
    control,
    name: 'search_config.summary',
  });
  const rerankId = useWatch({
    control,
    name: 'search_config.rerank_id',
  });
  const summaryLlmId = useWatch({
    control,
    name: 'search_config.llm_setting.llm_id',
  });

  useEffect(() => {
    if (!modelsFetched) return;
    if (rerankModelEnabled && rerankId) trigger('search_config.rerank_id');
    if (aiSummaryEnabled && summaryLlmId) {
      trigger('search_config.llm_setting.llm_id');
    }
  }, [
    trigger,
    modelsFetched,
    rerankModelEnabled,
    aiSummaryEnabled,
    rerankId,
    summaryLlmId,
  ]);

  return { rerankModelEnabled, aiSummaryEnabled };
};
