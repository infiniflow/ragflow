import {
  LlmSettingEnabledSchema,
  LlmSettingFieldSchema,
} from '@/components/llm-setting-items/next';
import { MetadataFilterSchema } from '@/components/metadata-filter';
import { rerankCandidatesCountSchema } from '@/components/rerank-candidates-count-item';
import { rerankFormSchema } from '@/components/rerank';
import {
  similarityThresholdSchema,
  vectorSimilarityWeightSchema,
} from '@/components/similarity-slider';
import { topnSchema } from '@/components/top-n-item';
import { WebSearchProvider } from '@/constants/chat';
import { useTranslate } from '@/hooks/common-hooks';
import { z, ZodIssueCode } from 'zod';
import { missingWebSearchApiKeyField } from '../web-search-api-key';
import { chatPromptKbIssues } from './validate-chat-prompt';

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
    brave_api_key: z.string().optional(),
    exa_api_key: z.string().optional(),
    firecrawl_api_key: z.string().optional(),
    linkup_api_key: z.string().optional(),
    parallel_api_key: z.string().optional(),
    querit_api_key: z.string().optional(),
    serply_api_key: z.string().optional(),
    tavily_api_key: z.string().optional(),
    youcom_api_key: z.string().optional(),
    web_search_provider: z
      .enum([
        WebSearchProvider.Brave,
        WebSearchProvider.Exa,
        WebSearchProvider.Firecrawl,
        WebSearchProvider.Linkup,
        WebSearchProvider.Parallel,
        WebSearchProvider.Querit,
        WebSearchProvider.Serply,
        WebSearchProvider.Tavily,
        WebSearchProvider.YouCom,
      ])
      .optional()
      .or(z.literal('')),
    reasoning: z.boolean().optional(),
    cross_languages: z.array(z.string()).optional(),
    reference_metadata: z
      .object({
        include: z.boolean().optional(),
        fields: z.array(z.string()).optional(),
      })
      .optional(),
  });

  const formSchema = z
    .object({
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
      ...rerankCandidatesCountSchema,
      ...MetadataFilterSchema,
    })
    .superRefine((value, ctx) => {
      for (const issue of chatPromptKbIssues(value, t)) {
        ctx.addIssue({
          code: ZodIssueCode.custom,
          path: issue.path,
          message: issue.message,
        });
      }

      // A keyed provider selected without its key fails SILENTLY at runtime —
      // the Internet switch never appears in the chat box — so block the save
      // here instead. Keyless providers (You.com) are exempt. The rule lives in
      // missingWebSearchApiKeyField so it can be unit-tested without the form.
      const missingKeyField = missingWebSearchApiKeyField(value?.prompt_config);
      if (missingKeyField) {
        ctx.addIssue({
          code: ZodIssueCode.custom,
          path: ['prompt_config', missingKeyField],
          message: t('webSearchApiKeyRequired'),
        });
      }
    });

  return formSchema;
}
