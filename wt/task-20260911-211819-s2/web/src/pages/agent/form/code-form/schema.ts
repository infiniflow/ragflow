import { ProgrammingLanguage } from '@/constants/agent';
import { z } from 'zod';
import { isValidCodeOutputName } from './utils';

export const FormSchema = z.object({
  lang: z.enum([ProgrammingLanguage.Python, ProgrammingLanguage.Javascript]),
  script: z.string(),
  arguments: z.array(z.object({ name: z.string(), type: z.string() })),
  output: z.object({
    name: z
      .string()
      .trim()
      .min(1, 'Name is required')
      .refine(
        isValidCodeOutputName,
        'Name cannot use reserved outputs or path syntax',
      ),
    type: z.string().trim().min(1, 'Type is required'),
  }),
});

export type FormSchemaType = z.infer<typeof FormSchema>;
