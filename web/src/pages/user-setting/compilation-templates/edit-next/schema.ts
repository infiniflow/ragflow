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

import { draftFromSections, validateDraft } from './ontology/model';

/**
 * The property-side issue codes, so a finding lands on the section it is about
 * instead of always on `entity`.
 */
const PropertyIssueCodes = [
  'property_without_type',
  'duplicate_property',
  'property_without_description',
  'invalid_kind',
  'missing_domain',
  'unknown_domain',
  'missing_range',
  'unknown_range',
  'unsupported_datatype',
];

export const buildSectionSchema = (t: (key: string) => string) =>
  z.object({
    description: z.string().optional(),
    fields: z
      .array(
        z.record(
          z.string().min(1, t('knowledgeCompilation.fieldDescriptionRequired')),
        ),
      )
      .min(1),
  });

export const buildRaptorConfigSchema = (t: (key: string) => string) =>
  z.object({
    prompt: z.string().optional(),
    claim_prompt: z.string().optional(),
    max_token: z
      .number()
      .min(512, t('knowledgeCompilation.maxTokenRequired'))
      .max(2048),
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

/**
 * Structured config the section editor carries through verbatim instead of
 * modelling as a field grid (see ConfigPassthroughKeys in utils.ts). It needs
 * its own union member so validation does not depend on buildSynthesisSchema
 * happening to accept an object with unrelated keys.
 */
export const buildPassthroughConfigSchema = () => z.object({}).passthrough();

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
          buildPassthroughConfigSchema(),
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

      // An ontology template's cross-references are what the service rejects on
      // save, and a rejection comes back as `code !== 0` with nothing on screen —
      // a silent dead button. The check runs here so the save is blocked while
      // the editor is already listing the same findings, and the reader is
      // looking at the reason.
      if (template.kind !== 'ontology') return;
      const config = template.config as {
        entity?: { fields?: Record<string, string>[] };
        relation?: { fields?: Record<string, string>[] };
      };
      validateDraft(draftFromSections(config?.entity, config?.relation)).forEach(
        (issue) => {
          context.addIssue({
            code: z.ZodIssueCode.custom,
            path: [
              'config',
              PropertyIssueCodes.includes(issue.code) ? 'relation' : 'entity',
            ],
            message: issue.message,
          });
        },
      );
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
