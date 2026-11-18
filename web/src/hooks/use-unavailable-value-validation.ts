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

import { ModelTypeMap } from '@/components/model-tree-select';
import { get } from 'lodash';
import { useEffect, useMemo } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { useCompilationTemplateGroupValidIds } from './use-compilation-template-group-request';
import { useModelValidIds } from './use-llm-request';

type UnavailableValueCheck = {
  fieldName: string;
  validIds: Set<string>;
  isFetched: boolean;
  message: string;
  getPendingMessage?: (value: string) => string | undefined;
};

/**
 * Extend a form schema with an availability check on one of its fields
 * (`fieldName`, a dotted path such as `llm_id`): once the referenced-option
 * list has loaded, a persisted value pointing at nothing the current user
 * can use gets flagged. Mirrors use-stale-dataset-validation.
 */
export function useUnavailableValueFormSchema<T extends z.ZodTypeAny>(
  schema: T,
  {
    fieldName,
    validIds,
    isFetched,
    message,
    getPendingMessage,
  }: UnavailableValueCheck,
) {
  const formSchema = useMemo(() => {
    const path = fieldName.split('.');
    return schema.superRefine((data: any, ctx) => {
      const value = get(data, path) as string | undefined;
      if (!value) return;
      const pendingMessage = getPendingMessage?.(value);
      if (pendingMessage) {
        ctx.addIssue({
          path,
          message: pendingMessage,
          code: z.ZodIssueCode.custom,
        });
      } else if (isFetched && !validIds.has(value)) {
        ctx.addIssue({
          path,
          message,
          code: z.ZodIssueCode.custom,
        });
      }
    });
  }, [schema, validIds, isFetched, fieldName, message, getPendingMessage]);

  return { formSchema };
}

/**
 * Availability check for a model field (default `llm_id`), validated against
 * the resource tenant and access-user scope. A shared canvas uses its owner's
 * resources, while imported DSL references use the importing user's scope.
 */
export function useUnavailableModelFormSchema<T extends z.ZodTypeAny>(
  schema: T,
  {
    fieldName = 'llm_id',
    modelTypes = ModelTypeMap.llm_id,
    resourceTenantId,
    accessUserId,
    ownerTenantId,
  }: {
    fieldName?: string;
    modelTypes?: string[];
    resourceTenantId?: string;
    accessUserId?: string;
    ownerTenantId?: string;
  } = {},
) {
  const { t } = useTranslation();
  const {
    validIds,
    isFetched: directModelsFetched,
    isSettled: modelsFetched,
    getPendingMessage,
  } = useModelValidIds(
    modelTypes,
    resourceTenantId ?? ownerTenantId,
    accessUserId,
  );

  const { formSchema } = useUnavailableValueFormSchema(schema, {
    fieldName,
    validIds,
    isFetched: directModelsFetched,
    getPendingMessage,
    message: t('common.modelUnavailable'),
  });

  return { formSchema, modelsFetched };
}

/**
 * Availability check for a compilation template group field (default
 * `compilation_template_group_id`), validated against the groups owned by
 * `accessUserId` (the current user when omitted). Groups resolve within the
 * active tenant at run time, so an imported dsl.json reference only exists
 * under its original author's account and gets flagged.
 */
export function useUnavailableCompilationTemplateGroupFormSchema<
  T extends z.ZodTypeAny,
>(
  schema: T,
  {
    fieldName = 'compilation_template_group_id',
    accessUserId,
  }: { fieldName?: string; accessUserId?: string } = {},
) {
  const { t } = useTranslation();
  const { validIds, isFetched: templateGroupsFetched } =
    useCompilationTemplateGroupValidIds(accessUserId);

  const { formSchema } = useUnavailableValueFormSchema(schema, {
    fieldName,
    validIds,
    isFetched: templateGroupsFetched,
    message: t('knowledgeConfiguration.compilationTemplateUnavailable'),
  });

  return { formSchema, templateGroupsFetched };
}

/**
 * A persisted value never fires onChange validation, so once the referenced
 * list has loaded, revalidate explicitly — the error should be visible before
 * submit. `form` must stay out of the effect deps (the provider recreates it
 * on every render, which loops a failing field); `trigger` is the stable
 * control method.
 */
export function useRevalidateUnavailableValue(
  form: UseFormReturn<any>,
  isFetched: boolean,
  fieldName: string,
) {
  const value = useWatch({ control: form.control, name: fieldName });
  const trigger = form.trigger;

  useEffect(() => {
    if (!isFetched || !value) {
      return;
    }

    trigger(fieldName);
  }, [trigger, isFetched, value, fieldName]);
}
