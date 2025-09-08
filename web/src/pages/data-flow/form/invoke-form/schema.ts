import { z } from 'zod';

export const VariableFormSchema = z.object({
  key: z.string(),
  ref: z.string(),
  value: z.string(),
});

export const FormSchema = z.object({
  url: z.string().url(),
  method: z.string(),
  timeout: z.number(),
  headers: z.string(),
  proxy: z.string().url(),
  clean_html: z.boolean(),
  variables: z.array(VariableFormSchema),
});

export type FormSchemaType = z.infer<typeof FormSchema>;

export type VariableFormSchemaType = z.infer<typeof VariableFormSchema>;
