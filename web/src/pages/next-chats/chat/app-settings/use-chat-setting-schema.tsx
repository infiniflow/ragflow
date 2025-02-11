import { useTranslate } from '@/hooks/common-hooks';
import { z } from 'zod';

export function useChatSettingSchema() {
  const { t } = useTranslate('chat');

  const promptConfigSchema = z.object({
    quote: z.boolean(),
    keyword: z.boolean(),
    tts: z.boolean(),
    empty_response: z.string().min(1, {
      message: t('emptyResponse'),
    }),
    prologue: z.string().min(1, {}),
    system: z.string().min(1, { message: t('systemMessage') }),
    refine_multiturn: z.boolean(),
    use_kg: z.boolean(),
  });

  const formSchema = z.object({
    name: z.string().min(1, { message: t('assistantNameMessage') }),
    icon: z.array(z.instanceof(File)),
    language: z.string().min(1, {
      message: 'Username must be at least 2 characters.',
    }),
    description: z.string(),
    kb_ids: z.array(z.string()).min(0, {
      message: 'Username must be at least 1 characters.',
    }),
    prompt_config: promptConfigSchema,
    top_n: z.number(),
    vector_similarity_weight: z.number(),
    top_k: z.number(),
  });

  return formSchema;
}
