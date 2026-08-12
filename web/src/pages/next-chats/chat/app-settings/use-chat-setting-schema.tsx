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
import { WebSearchProvider } from '@/constants/chat';
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
    parameters: z
      .array(
        z.object({
          key: z.string().min(1, { message: t('variableKeyMessage') }),
          optional: z.boolean(),
        }),
      )
      .optional(),
    tavily_api_key: z.string().optional(),
    querit_api_key: z.string().optional(),
    web_search_provider: z
      .enum([WebSearchProvider.Tavily, WebSearchProvider.Querit])
      .optional(),
    reasoning: z.boolean().optional(),
    cross_languages: z.array(z.string()).optional(),
    reference_metadata: z
      .object({
        include: z.boolean().optional(),
        fields: z.array(z.string()).optional(),
      })
      .optional(),
  });

  const formSchema = z.object({
    name: z.string().min(1, { message: t('assistantNameMessage') }),
    icon: z.string(),
    description: z.string().optional(),
    dataset_ids: z.array(z.string()).min(0, {
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
  });

  return formSchema;
}
