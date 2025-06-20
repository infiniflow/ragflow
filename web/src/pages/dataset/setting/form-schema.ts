import { z } from 'zod';

export const formSchema = z.object({
  name: z.string().min(1, {
    message: 'Username must be at least 2 characters.',
  }),
  description: z.string().min(2, {
    message: 'Username must be at least 2 characters.',
  }),
  // avatar: z.instanceof(File),
  avatar: z.any().nullish(),
  permission: z.string(),
  parser_id: z.string(),
  embd_id: z.string(),
  parser_config: z
    .object({
      layout_recognize: z.string(),
      chunk_token_num: z.number(),
      delimiter: z.string(),
      auto_keywords: z.number().optional(),
      auto_questions: z.number().optional(),
      html4excel: z.boolean(),
      tag_kb_ids: z.array(z.string()).nullish(),
      topn_tags: z.number().optional(),
      raptor: z
        .object({
          use_raptor: z.boolean().optional(),
          prompt: z.string().optional(),
          max_token: z.number().optional(),
          threshold: z.number().optional(),
          max_cluster: z.number().optional(),
          random_seed: z.number().optional(),
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
    })
    .optional(),
  pagerank: z.number(),
  // icon: z.array(z.instanceof(File)),
});
