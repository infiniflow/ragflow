import { z } from 'zod';

export const FormSchema = z.object({
  loop_variables: z.array(
    z.object({
      variable: z.string().optional(),
      type: z.string().optional(),
      value: z.string().or(z.number()).or(z.boolean()).optional(),
      input_mode: z.string(),
    }),
  ),
  logical_operator: z.string(),
  loop_termination_condition: z.array(
    z.object({
      variable: z.string().optional(),
      operator: z.string().optional(),
      value: z.string().or(z.number()).or(z.boolean()).optional(),
      input_mode: z.string().optional(),
    }),
  ),
  maximum_loop_count: z.number(),
});

export type LoopFormSchemaType = z.infer<typeof FormSchema>;
