import { z } from 'zod';

export const VariableAggregatorSchema = {
  groups: z.array(
    z.object({
      group_name: z.string(),
      variables: z.array(z.object({ value: z.string().optional() })),
      type: z.string().optional(),
    }),
  ),
};

export const FormSchema = z.object(VariableAggregatorSchema);

export type VariableAggregatorFormSchemaType = z.infer<typeof FormSchema>;
