import { LlmSettingSchema } from '@/components/llm-setting-items/next';
import i18n from '@/locales/config';
import { z } from 'zod';

// A group switch persists as a boolean from the form UI but as 0/1 in
// legacy saved configs; the form UI normalizes with the same check.
const isGroupEnabled = (v: number | boolean | undefined) =>
  v === true || v === 1;

export const FormSchema = z
  .object({
    keywords: z
      .object({
        enabled: z.union([z.number(), z.boolean()]).optional(),
        top_n: z.number().optional(),
        system_prompt: z.string().optional(),
      })
      .optional(),
    questions: z
      .object({
        enabled: z.union([z.number(), z.boolean()]).optional(),
        top_n: z.number().optional(),
        system_prompt: z.string().optional(),
      })
      .optional(),
    tags: z
      .object({
        enabled: z.union([z.number(), z.boolean()]).optional(),
        top_n: z.number().optional(),
        tag_file_id: z.string().optional(),
      })
      .optional(),
    summary: z
      .object({
        enabled: z.union([z.number(), z.boolean()]).optional(),
        system_prompt: z.string().optional(),
      })
      .optional(),
    // Matches schema.ExtractorParam.Metadata on the Go side
    // (internal/ingestion/component/schema/extractor.go).
    metadata: z
      .object({
        enabled: z.union([z.number(), z.boolean()]).optional(),
        metadata: z.any().optional(),
        built_in_metadata: z.any().optional(),
      })
      .optional(),
    ...LlmSettingSchema,
    // No required mark on the model field — an LLM is optional for extraction.
    llm_id: z.string().optional(),
  })
  .superRefine((data, ctx) => {
    // Same enabled-with-nothing-to-extract rules as the canvas checklist
    // (pages/agent/utils/canvas-checklist.ts): a group switched on with
    // top_n < 1 fails the Go canvas build at parse time, and tags without
    // a tag file silently skip — catch both at save time instead.
    if (isGroupEnabled(data.keywords?.enabled)) {
      if (!(Number(data.keywords?.top_n) > 0)) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['keywords', 'top_n'],
          message: i18n.t('flow.extractorKeywordsTopNMissing'),
        });
      }
    }
    if (isGroupEnabled(data.questions?.enabled)) {
      if (!(Number(data.questions?.top_n) > 0)) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['questions', 'top_n'],
          message: i18n.t('flow.extractorQuestionsTopNMissing'),
        });
      }
    }
    if (isGroupEnabled(data.tags?.enabled)) {
      if (!(Number(data.tags?.top_n) > 0)) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['tags', 'top_n'],
          message: i18n.t('flow.extractorTagsTopNMissing'),
        });
      }
      if (!data.tags?.tag_file_id) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['tags', 'tag_file_id'],
          message: i18n.t('flow.extractorTagFileMissing'),
        });
      }
    }
  });

export type ExtractorFormSchemaType = z.infer<typeof FormSchema>;
