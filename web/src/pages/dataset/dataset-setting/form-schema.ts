import { t } from 'i18next';
import { z } from 'zod';

export const formSchema = z
  .object({
    parseType: z.number(),
    name: z.string().min(1, {
      message: 'Username must be at least 2 characters.',
    }),
    description: z.string().min(2, {
      message: 'Username must be at least 2 characters.',
    }),
    // avatar: z.instanceof(File),
    avatar: z.any().nullish(),
    permission: z.string().optional(),
    language: z.string().optional(),
    parser_id: z.string(),
    pipeline_id: z.string().optional(),
    pipeline_name: z.string().optional(),
    pipeline_avatar: z.string().optional(),
    embd_id: z.string(),
    parser_config: z
      .object({
        layout_recognize: z.string(),
        chunk_token_num: z.number(),
        delimiter: z.string(),
        enable_children: z.boolean(),
        children_delimiter: z.string(),
        auto_keywords: z.number().optional(),
        auto_questions: z.number().optional(),
        html4excel: z.boolean(),
        tag_kb_ids: z.array(z.string()).nullish(),
        topn_tags: z.number().optional(),
        toc_extraction: z.boolean().optional(),
        image_table_context_window: z.number().optional(),
        overlapped_percent: z.number().optional(),
        // MinerU-specific options
        mineru_parse_method: z.enum(['auto', 'txt', 'ocr']).optional(),
        mineru_formula_enable: z.boolean().optional(),
        mineru_table_enable: z.boolean().optional(),
        mineru_lang: z.string().optional(),
        raptor: z
          .object({
            use_raptor: z.boolean().optional(),
            prompt: z.string().optional(),
            max_token: z.number().optional(),
            threshold: z.number().optional(),
            max_cluster: z.number().optional(),
            random_seed: z.number().optional(),
            scope: z.string().optional(),
          })
          .refine(
            (data) => {
              if (data.use_raptor && !data.prompt) {
                return false;
              }
              return true;
            },
            {
              message: 'Prompt is required',
              path: ['prompt'],
            },
          ),
        graphrag: z
          .object({
            use_graphrag: z.boolean().optional(),
            entity_types: z.array(z.string()).optional(),
            method: z.string().optional(),
            resolution: z.boolean().optional(),
            community: z.boolean().optional(),
          })
          .refine(
            (data) => {
              if (
                data.use_graphrag &&
                (!data.entity_types || data.entity_types.length === 0)
              ) {
                return false;
              }
              return true;
            },
            {
              message: 'Please enter Entity types',
              path: ['entity_types'],
            },
          ),
        metadata: z.any().optional(),
        built_in_metadata: z
          .array(
            z.object({
              key: z.string().optional(),
              type: z.string().optional(),
            }),
          )
          .optional(),
        enable_metadata: z.boolean().optional(),
        llm_id: z.string().optional(),
      })
      .optional(),
    pagerank: z.number(),
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
    // icon: z.array(z.instanceof(File)),
  })
  .superRefine((data, ctx) => {
    if (data.parseType === 2 && !data.pipeline_id) {
      ctx.addIssue({
        path: ['pipeline_id'],
        message: t('common.pleaseSelect'),
        code: 'custom',
      });
    }
  });

export const pipelineFormSchema = z.object({
  pipeline_id: z.string().optional(),
  set_default: z.boolean().optional(),
  file_filter: z.string().optional(),
});

// export const linkPiplineFormSchema = pipelineFormSchema.pick({
//   pipeline_id: true,
//   file_filter: true,
// });
// export const editPiplineFormSchema = pipelineFormSchema.pick({
//   set_default: true,
//   file_filter: true,
// });
