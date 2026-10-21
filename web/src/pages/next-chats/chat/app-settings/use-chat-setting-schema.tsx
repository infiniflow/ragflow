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
import { useMemo } from 'react';
import { z, ZodIssueCode } from 'zod';
import { missingWebSearchApiKeyField } from '../web-search-api-key';
import { ontologyBindingIssues } from './ontology-binding';
import { chatPromptKbIssues } from './validate-chat-prompt';

// Pure builder so the schema can be unit-tested without rendering a form.
// All settings remain in one schema because both naive and agentic fields are
// available in the settings form at the same time.
export function buildChatSettingFormSchema(t: (key: string) => string) {
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
    // null is a value here, not a stand-in for "absent": the server merges the
    // patch into the stored prompt_config key by key, so clearing the
    // assistant's knowledge-layer override takes an explicit null. The shape is
    // declared even when the key is unset so the resolver does not strip what
    // the panel wrote.
    ontology: z
      .object({
        enabled: z.boolean().optional(),
        model_id: z.string().optional(),
        model_version: z.number().optional(),
        knowledge_dataset_ids: z.array(z.string()).optional(),
      })
      .nullable()
      .optional(),
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
      // Empty means "inherit the tenant default": most chats never override
      // the decision model, and clearing the field must be expressible.
      decision_id: z.string().optional(),
      ...keywordsSimilarityWeightSchema,
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

      // A half-written knowledge-layer binding resolves to no binding at all,
      // server-side and in silence — a model with no scope has nowhere to look.
      // Refusing the save is the only place that person finds out before asking
      // a question and getting an answer that quietly read nothing.
      for (const issue of ontologyBindingIssues(value, t)) {
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

export function useChatSettingSchema() {
  const { t } = useTranslate('chat');

  return useMemo(
    () => buildChatSettingFormSchema((key: string) => t(key)),
    [t],
  );
}
