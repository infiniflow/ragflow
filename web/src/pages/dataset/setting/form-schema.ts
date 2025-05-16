import { z } from 'zod';

export const formSchema = z.object({
  name: z.string().min(1, {
    message: 'Username must be at least 2 characters.',
  }),
  description: z.string().min(2, {
    message: 'Username must be at least 2 characters.',
  }),
  avatar: z.instanceof(File),
  permission: z.string(),
  parser_id: z.string(),
  embd_id: z.string(),
  parser_config: z.object({
    layout_recognize: z.string(),
    chunk_token_num: z.number(),
    delimiter: z.string(),
    auto_keywords: z.number(),
    auto_questions: z.number(),
    html4excel: z.boolean(),
    tag_kb_ids: z.array(z.string()),
    topn_tags: z.number(),
    raptor: z.object({
      use_raptor: z.boolean(),
      prompt: z.string(),
      max_token: z.number(),
      threshold: z.number(),
      max_cluster: z.number(),
      random_seed: z.number(),
    }),
    graphrag: z.object({
      use_graphrag: z.boolean(),
      entity_types: z.array(z.string()),
      method: z.string(),
      resolution: z.boolean(),
      community: z.boolean(),
    }),
  }),
  pagerank: z.number(),
  // icon: z.array(z.instanceof(File)),
});
