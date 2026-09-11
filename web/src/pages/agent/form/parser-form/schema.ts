import i18n from '@/locales/config';
import { z } from 'zod';
import { FileType } from '../../constant/pipeline';

export const SetupSchema = z
  .object({
    fileFormat: z.string().nullish(),
    // preprocess: z.array(z.string()).optional(),
    output_format: z.string().optional(),
    parse_method: z.string().optional(),
    lang: z.string().optional(),
    fields: z.array(z.string()).optional(),
    vlm: z.object({ llm_id: z.string().optional() }).optional(),
    flatten_media_to_text: z.boolean().optional(),
    system_prompt: z.string().optional(),
    table_result_type: z.string().optional(),
    markdown_image_response_type: z.string().optional(),
    enable_multi_column: z.boolean().optional(),
    remove_toc: z.boolean().optional(),
    remove_header_footer: z.boolean().optional(),
    pages: z
      .array(
        z
          .object({
            // Keep these checks on the fields themselves: an object-level
            // `superRefine` is skipped whenever the base shape fails to
            // parse, so one missing sibling would silently swallow the
            // other field's error.
            from: z.coerce
              .number()
              .int(i18n.t('knowledgeDetails.pageRangeFromInvalid'))
              .min(1, i18n.t('knowledgeDetails.pageRangeFromInvalid')),
            to: z.coerce
              .number()
              .int(i18n.t('knowledgeDetails.pageRangeToInvalid'))
              .min(1, i18n.t('knowledgeDetails.pageRangeToInvalid')),
          })
          .refine(({ from, to }) => to >= from, {
            path: ['to'],
            message: i18n.t('knowledgeDetails.pageRangeToInvalid'),
          }),
      )
      .optional(),
  })
  .superRefine((values, ctx) => {
    if (values.fileFormat === FileType.Email && !values.fields?.length) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['fields'],
        message: 'Fields is required',
      });
    }
  });

export const FormSchema = z.object({
  setups: z.array(SetupSchema).min(1, i18n.t('flow.atLeastOneFileType')),
});

export type ParserFormSchemaType = z.infer<typeof FormSchema>;
