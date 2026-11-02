import { addParserConfigIssues } from '@/components/pipeline-operator-tabs/parser-config-validation';
import { DESCRIPTION_MAX_LENGTH } from '@/constants/common';
import { ParseType } from '@/constants/knowledge';
import { t } from 'i18next';
import { z } from 'zod';

// The missing verdict arrives as a getter, not a value: the schema must exist
// before useForm runs, but whether the saved parser option still exists can
// only be derived from the form's watched values plus the lazily loaded
// parser tree. Reading the verdict through a ref-backed getter keeps every
// validation run on the latest state without rebuilding the resolver.
export const buildFormSchema = (isParserOptionMissing: () => boolean) =>
  z
    .object({
      parse_type: z.nativeEnum(ParseType).optional(),
      pipeline_id: z.string().optional(),
      // Pinned dataflow canvas version (design section 5/C2). Empty means
      // track the canvas latest version.
      pipeline_version_id: z.string().optional(),
      name: z.string().min(1, {
        message: 'Username must be at least 2 characters.',
      }),
      description: z
        .string()
        .max(DESCRIPTION_MAX_LENGTH, {
          message: t('common.descriptionMaxLength', {
            max: DESCRIPTION_MAX_LENGTH,
          }),
        })
        .optional(),
      parser_id: z.string().optional(),
      avatar: z.any().nullish(),
      permission: z.string().optional(),
      language: z.string().optional(),
      embedding_model: z.string(),
      pagerank: z.number(),
      parser_config: z.record(z.string(), z.any()).optional(),
      connectors: z
        .array(
          z.object({
            id: z.string().optional(),
            name: z.string().optional(),
            source: z.string().optional(),
            ststus: z.string().optional(),
            auto_parse: z.string().optional(),
          }),
        )
        .optional(),
    })
    .superRefine((data, ctx) => {
      if (data.parse_type === ParseType.Pipeline && !data.pipeline_id) {
        ctx.addIssue({
          path: ['pipeline_id'],
          message: t('common.pleaseSelect'),
          code: 'custom',
        });
      }
      if (data.parse_type === ParseType.BuiltIn && !data.parser_id?.trim()) {
        ctx.addIssue({
          path: ['parser_id'],
          message: t('common.pleaseSelect'),
          code: 'custom',
        });
      }
      if (isParserOptionMissing()) {
        ctx.addIssue({
          path: [
            data.parse_type === ParseType.Pipeline
              ? 'pipeline_id'
              : 'parser_id',
          ],
          message: t('knowledgeConfiguration.parserOptionUnavailableMessage'),
          code: 'custom',
        });
      }
      addParserConfigIssues(data.parser_config, ctx, t);
    });

export type DatasetSettingFormValues = z.infer<
  ReturnType<typeof buildFormSchema>
>;
