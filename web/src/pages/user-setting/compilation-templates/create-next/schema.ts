import { z } from 'zod';

export const buildSectionSchema = (t: (key: string) => string) =>
  z.object({
    description: z.string().optional(),
    fields: z
      .array(z.record(z.string().min(1, t('setting.fieldDescriptionRequired'))))
      .min(1),
  });

export const buildRaptorConfigSchema = (t: (key: string) => string) =>
  z.object({
    prompt: z.string().optional(),
    max_token: z.number().min(1, t('setting.maxTokenRequired')),
    threshold: z.number().min(0).max(1),
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
  z.object({
    id: z.string().optional(),
    name: z.string().min(1, t('setting.templateNameRequired')),
    description: z.string().optional(),
    llm_id: z.string().min(1, t('setting.llmForExtractionRequired')),
    kind: z.string().min(1, t('setting.templateKindRequired')),
    config: z.record(
      z.union([
        buildRaptorConfigSchema(t),
        buildSectionSchema(t),
        buildSynthesisSchema(),
        z.string(),
        z.boolean(),
      ]),
    ),
  });

export const buildFormSchema = (t: (key: string) => string) =>
  z.object({
    name: z.string().min(1, t('setting.groupNameRequired')),
    description: z.string().optional(),
    avatar: z.string().optional(),
    templates: z.array(buildTemplateSchema(t)).min(1),
  });

export type TemplateSchemaType = z.infer<
  ReturnType<typeof buildTemplateSchema>
>;
export type FormSchemaType = z.infer<ReturnType<typeof buildFormSchema>>;
