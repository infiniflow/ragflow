import { z } from 'zod';

export const formSchema = z.object({
  name: z.string().min(1, {
    message: 'Username must be at least 2 characters.',
  }),
  description: z.string().optional(),
  avatar: z.any().nullish(),
  permission: z.string().optional(),
  embedding_model: z.string(),
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
});
