import {
  LlmSettingEnabledSchema,
  LlmSettingFieldSchema,
} from '@/components/llm-setting-items/next';
import { MetadataFilterSchema } from '@/components/metadata-filter';
import { rerankCandidatesCountSchema } from '@/components/rerank-candidates-count-item';
import { rerankFormSchema } from '@/components/rerank';
import {
  similarityThresholdSchema,
  keywordsSimilarityWeightSchema,
} from '@/components/similarity-slider';
import { topnSchema } from '@/components/top-n-item';
import { WebSearchProvider } from '@/constants/chat';
import { useTranslate } from '@/hooks/common-hooks';
import { ThinkingMode } from '@/hooks/use-thinking-level';
import { useMemo } from 'react';
import { z, ZodIssueCode } from 'zod';
import { missingWebSearchApiKeyField } from '../web-search-api-key';
import { chatPromptKbIssues } from './validate-chat-prompt';

// Pure builder so the schema can be unit-tested without rendering a form.
// Fields hidden in the current mode stay declared (their values round-trip
// through saves); only the validation rules are mode-conditional.
export function buildChatSettingFormSchema(
  mode: ThinkingMode,
  t: (key: string) => string,
) {
  const isAgentic = mode === ThinkingMode.Agentic;

  const promptConfigSchema = z.object({
    quote: z.boolean(),
    keyword: z.boolean(),
    tts: z.boolean(),
    empty_response: z.string().optional(),
    prologue: z.string().optional(),
    system: isAgentic
      ? z.string()
      : z.string().min(1, { message: t('systemMessage') }),
    refine_multiturn: z.boolean(),
    parameters: z
      .array(
        z.object({
          key: isAgentic
            ? z.string()
            : z.string().min(1, { message: t('variableKeyMessage') }),
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
    search1api_api_key: z.string().optional(),
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
        WebSearchProvider.Search1API,
        WebSearchProvider.Serply,
        WebSearchProvider.Tavily,
        WebSearchProvider.YouCom,
      ])
      .optional()
      .or(z.literal('')),
    reasoning: z.boolean().optional(),
    cross_languages: z.array(z.string()).optional(),
    // The per-chat agentic template override lives here; without this
    // declaration zod strips the key at parse time and the full-replace PUT
    // would silently delete a saved override on every settings save.
    agentic_rag: z
      .object({
        content: z
          .string()
          .max(32768, { message: t('agenticTemplateTooLong') })
          .optional(),
        tools: z.array(z.string()).optional(),
      })
      .nullable()
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
      ...keywordsSimilarityWeightSchema,
      ...similarityThresholdSchema,
      ...topnSchema,
      ...rerankCandidatesCountSchema,
      ...MetadataFilterSchema,
    })
    .superRefine((value, ctx) => {
      // The knowledge-placeholder and empty-response rules only apply to
      // fields shown in naive mode; agentic runs its own template instead.
      if (!isAgentic) {
        for (const issue of chatPromptKbIssues(value, t)) {
          ctx.addIssue({
            code: ZodIssueCode.custom,
            path: issue.path,
            message: issue.message,
          });
        }
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

export function useChatSettingSchema(mode: ThinkingMode) {
  const { t } = useTranslate('chat');

  return useMemo(
    () => buildChatSettingFormSchema(mode, (key: string) => t(key)),
    [mode, t],
  );
}
