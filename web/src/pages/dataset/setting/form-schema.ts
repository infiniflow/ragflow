import { ParseType } from '@/constants/knowledge';
import { t } from 'i18next';
import { z } from 'zod';

export const formSchema = z
  .object({
    parse_type: z.nativeEnum(ParseType).optional(),
    pipeline_id: z.string().optional(),
    pipeline_name: z.string().optional(),
    pipeline_avatar: z.string().optional(),
    name: z.string().min(1, {
      message: 'Username must be at least 2 characters.',
    }),
    description: z.string().optional(),
    parser_id: z.string().optional(),
    avatar: z.any().nullish(),
    permission: z.string().optional(),
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
  });
