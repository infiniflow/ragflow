import {
  LlmSettingEnabledSchema,
  LlmSettingFieldSchema,
} from '@/components/llm-setting-items/next';
import { MetadataFilterSchema } from '@/components/metadata-filter';
import { rerankFormSchema } from '@/components/rerank';
import {
  similarityThresholdSchema,
  vectorSimilarityWeightSchema,
} from '@/components/similarity-slider';
import { topnSchema } from '@/components/top-n-item';
import { useTranslate } from '@/hooks/common-hooks';
import { z } from 'zod';

export function useChatSettingSchema() {
  const { t } = useTranslate('chat');

  const promptConfigSchema = z.object({
    quote: z.boolean(),
    keyword: z.boolean(),
    tts: z.boolean(),
    empty_response: z.string().optional(),
    prologue: z.string().optional(),
    system: z.string().min(1, { message: t('systemMessage') }),
    refine_multiturn: z.boolean(),
    use_kg: z.boolean(),
    parameters: z
      .array(
        z.object({
          key: z.string(),
          optional: z.boolean(),
        }),
      )
      .optional(),
    tavily_api_key: z.string().optional(),
    reasoning: z.boolean().optional(),
    cross_languages: z.array(z.string()).optional(),
    toc_enhance: z.boolean().optional(),
  });

  const formSchema = z
    .object({
      name: z.string().min(1, { message: t('assistantNameMessage') }),
      icon: z.string(),
      description: z.string().optional(),
      kb_ids: z.array(z.string()).min(0, {
        message: t('knowledgeBasesMessage'),
      }),
      prompt_config: promptConfigSchema,
      ...rerankFormSchema,
      llm_setting: z.object(LlmSettingFieldSchema),
      ...LlmSettingEnabledSchema,
      llm_id: z.string().optional(),
      ...vectorSimilarityWeightSchema,
      ...similarityThresholdSchema,
      ...topnSchema,
      ...MetadataFilterSchema,
    })
    .superRefine((values, ctx) => {
      const systemPrompt = values.prompt_config?.system ?? '';
      const hasKnowledgePlaceholder = systemPrompt.includes('{knowledge}');
      const hasKb = (values.kb_ids ?? []).length > 0;
      const hasTavily = !!values.prompt_config?.tavily_api_key;

      if (hasKnowledgePlaceholder && !hasKb && !hasTavily) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: t('knowledgeBasesMessage'),
          path: ['kb_ids'],
        });
      }
    });

  return formSchema;
}
