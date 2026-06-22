import { z } from 'zod';

export const buildSectionSchema = (t: (key: string) => string) =>
  z.object({
    description: z.string().optional(),
    fields: z
      .array(z.record(z.string().min(1, t('setting.fieldDescriptionRequired'))))
      .min(1),
  });

export const buildFormSchema = (t: (key: string) => string) =>
  z.object({
    name: z.string().min(1, t('setting.templateNameRequired')),
    description: z.string().optional(),
    llm_id: z.string().min(1, t('setting.llmForExtractionRequired')),
    kind: z.string().min(1, t('setting.templateKindRequired')),
    config: z.record(z.union([buildSectionSchema(t), z.string()])),
  });

export type FormSchemaType = z.infer<ReturnType<typeof buildFormSchema>>;
