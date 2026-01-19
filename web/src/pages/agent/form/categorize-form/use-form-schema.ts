import { LlmSettingSchema } from '@/components/llm-setting-items/next';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';

export function useCreateCategorizeFormSchema() {
  const { t } = useTranslation();

  const FormSchema = z.object({
    query: z.string().optional(),
    parameter: z.string().optional(),
    ...LlmSettingSchema,
    message_history_window_size: z.coerce.number(),
    items: z.array(
      z
        .object({
          name: z.string().min(1, t('flow.nameMessage')).trim(),
          description: z.string().optional(),
          uuid: z.string(),
          examples: z
            .array(
              z.object({
                value: z.string(),
              }),
            )
            .optional(),
        })
        .optional(),
    ),
  });

  return FormSchema;
}
