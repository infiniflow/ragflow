import { ProgrammingLanguage } from '@/constants/agent';
import { z } from 'zod';

export const FormSchema = z.object({
  lang: z.enum([ProgrammingLanguage.Python, ProgrammingLanguage.Javascript]),
  script: z.string(),
  arguments: z.array(z.object({ name: z.string(), type: z.string() })),
  outputs: z.union([
    z.array(z.object({ name: z.string(), type: z.string() })).optional(),
    z.object({ name: z.string(), type: z.string() }),
  ]),
});

export type FormSchemaType = z.infer<typeof FormSchema>;
