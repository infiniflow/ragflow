import { WebhookAlgorithmList } from '@/constants/agent';
import { z } from 'zod';

export const BeginFormSchema = z.object({
  enablePrologue: z.boolean().optional(),
  prologue: z.string().trim().optional(),
  mode: z.string(),
  inputs: z
    .array(
      z.object({
        key: z.string(),
        type: z.string(),
        value: z.string(),
        optional: z.boolean(),
        name: z.string(),
        options: z.array(z.union([z.number(), z.string(), z.boolean()])),
      }),
    )
    .optional(),
  methods: z.array(z.string()).optional(),
  content_types: z.string().optional(),
  security: z
    .object({
      auth_type: z.string(),
      ip_whitelist: z.array(z.object({ value: z.string() })),
      rate_limit: z.object({
        limit: z.number(),
        per: z.string().optional(),
      }),
      max_body_size: z.string(),
      jwt: z
        .object({
          algorithm: z.string().default(WebhookAlgorithmList[0]).optional(),
        })
        .optional(),
    })
    .optional(),
  schema: z
    .object({
      query: z
        .array(
          z.object({
            key: z.string(),
            type: z.string(),
            required: z.boolean(),
          }),
        )
        .optional(),
      headers: z
        .array(
          z.object({
            key: z.string(),
            type: z.string(),
            required: z.boolean(),
          }),
        )
        .optional(),
      body: z
        .array(
          z.object({
            key: z.string(),
            type: z.string(),
            required: z.boolean(),
          }),
        )
        .optional(),
    })
    .optional(),
  response: z
    .object({
      status: z.number(),
      // headers_template: z.array(
      //   z.object({ key: z.string(), value: z.string() }),
      // ),
      body_template: z.string().optional(),
    })
    .optional(),
  execution_mode: z.string().optional(),
});

export type BeginFormSchemaType = z.infer<typeof BeginFormSchema>;
