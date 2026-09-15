import { WebhookJWTAlgorithmList } from '@/constants/agent';
import { countBy } from 'lodash';
import { z } from 'zod';
import { isValidIpOrCidr } from './utils';

function validateUniqueKeys(
  items: Array<{ key: string }>,
  ctx: z.RefinementCtx,
) {
  const keyCounts = countBy(items, 'key');

  items.forEach((item, index) => {
    if (item.key && keyCounts[item.key] > 1) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: [index, 'key'],
        message: 'The key cannot be repeated!',
      });
    }
  });
}

const WebhookParametersSchema = z
  .array(
    z.object({
      key: z.string().trim().min(1, 'The key is required!'),
      type: z.string(),
      required: z.boolean(),
    }),
  )
  .superRefine(validateUniqueKeys)
  .optional();

export const BeginFormSchema = z.object({
  enablePrologue: z.boolean().optional(),
  prologue: z.string().trim().optional(),
  mode: z.string(),
  layout_recognize: z.string().optional(),
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
      ip_whitelist: z.array(
        z.object({
          value: z.string().trim().refine(isValidIpOrCidr, {
            message: 'Invalid IP address or CIDR block',
          }),
        }),
      ),
      rate_limit: z.object({
        limit: z.number(),
        per: z.string().optional(),
      }),
      max_body_size: z.string(),
      allow_anonymous: z.boolean().optional(),
      jwt: z
        .object({
          algorithm: z.string().default(WebhookJWTAlgorithmList[0]).optional(),
          required_claims: z.array(z.object({ value: z.string() })),
        })
        .optional(),
      hmac: z
        .object({
          header: z.string().optional(),
          secret: z.string().optional(),
        })
        .optional(),
    })
    .optional(),
  schema: z
    .object({
      query: WebhookParametersSchema,
      headers: WebhookParametersSchema,
      body: WebhookParametersSchema,
    })
    .optional(),
  response: z
    .object({
      status: z
        .number({ invalid_type_error: 'The status is required!' })
        .int('The status must be an integer'),
      // headers_template: z.array(
      //   z.object({ key: z.string(), value: z.string() }),
      // ),
      body_template: z.string().optional(),
    })
    .optional(),
  execution_mode: z.string().optional(),
});

export type BeginFormSchemaType = z.infer<typeof BeginFormSchema>;
