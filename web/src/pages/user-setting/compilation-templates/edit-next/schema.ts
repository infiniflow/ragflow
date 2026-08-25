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

import { z } from 'zod';

export const buildSectionSchema = (t: (key: string) => string) =>
  z.object({
    description: z.string().optional(),
    fields: z
      .array(z.record(z.string().min(1, t('knowledgeCompilation.fieldDescriptionRequired'))))
      .min(1),
  });

export const buildRaptorConfigSchema = (t: (key: string) => string) =>
  z.object({
    prompt: z.string().optional(),
    max_token: z.number().min(512, t('knowledgeCompilation.maxTokenRequired')).max(2048),
    clustering_threshold: z.number().min(0).max(1),
    clustering_ratio: z.number().min(0).max(1),
    rechunk: z.boolean().optional(),
  });

export const buildSynthesisSchema = () =>
  z
    .object({
      compile_kwd: z.string().optional(),
      enabled: z.boolean().optional(),
      example: z.string().optional(),
    })
    .passthrough();

export const buildTemplateSchema = (t: (key: string) => string) =>
  z
    .object({
      id: z.string().optional(),
      name: z.string().min(1, t('knowledgeCompilation.templateNameRequired')),
      description: z.string().optional(),
      kind: z.string().min(1, t('knowledgeCompilation.templateKindRequired')),
      config: z.record(
        z.union([
          buildRaptorConfigSchema(t),
          buildSectionSchema(t),
          buildSynthesisSchema(),
          z.string(),
          z.boolean(),
        ]),
      ),
    })
    .superRefine((template, context) => {
      if (
        template.kind === 'wiki' &&
        !['entity', 'topic'].includes(String(template.config.mode))
      ) {
        context.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['config', 'mode'],
          message: t('knowledgeCompilation.wikiModeRequired'),
        });
      }
    });

export const buildFormSchema = (t: (key: string) => string) =>
  z.object({
    name: z.string().optional(),
    description: z.string().optional(),
    avatar: z.string().optional(),
    templates: z.array(buildTemplateSchema(t)).min(1),
  });

export type TemplateSchemaType = z.infer<
  ReturnType<typeof buildTemplateSchema>
>;
export type FormSchemaType = z.infer<ReturnType<typeof buildFormSchema>>;
