import {
  DynamicForm,
  FormFieldConfig,
  FormFieldType,
} from '@/components/dynamic-form';
import { Button } from '@/components/ui/button';
import { Modal } from '@/components/ui/modal/modal';
import { LLMFactory } from '@/constants/llm';
import { useCommonTranslation, useTranslate } from '@/hooks/common-hooks';
import { useBuildModelTypeOptions } from '@/hooks/logic-hooks/use-build-options';
import { useFetchFactoryModels } from '@/hooks/use-llm-request';
import { IModalProps } from '@/interfaces/common';
import { IDynamicModel } from '@/interfaces/database/llm';
import { IAddLlmRequestBody } from '@/interfaces/request/llm';
import { useEffect, useMemo, useRef, useState } from 'react';
import { FieldValues, useFormContext } from 'react-hook-form';
import { LLMHeader } from '../../components/llm-header';

const llmFactoryToUrlMap: Partial<Record<LLMFactory, string>> = {
  [LLMFactory.Ollama]:
    'https://github.com/infiniflow/ragflow/blob/main/docs/guides/models/deploy_local_llm.mdx',
  [LLMFactory.Xinference]:
    'https://inference.readthedocs.io/en/latest/user_guide',
  [LLMFactory.ModelScope]:
    'https://www.modelscope.cn/docs/model-service/API-Inference/intro',
  [LLMFactory.LocalAI]: 'https://localai.io/docs/getting-started/models/',
  [LLMFactory.LMStudio]: 'https://lmstudio.ai/docs/basics',
  [LLMFactory.OpenAiAPICompatible]:
    'https://platform.openai.com/docs/models/gpt-4',
  [LLMFactory.TogetherAI]: 'https://docs.together.ai/docs/deployment-options',
  [LLMFactory.Replicate]: 'https://replicate.com/docs/topics/deployments',
  [LLMFactory.OpenRouter]: 'https://openrouter.ai/docs',
  [LLMFactory.HuggingFace]:
    'https://huggingface.co/docs/text-embeddings-inference/quick_tour',
  [LLMFactory.GPUStack]: 'https://docs.gpustack.ai/latest/quickstart',
  [LLMFactory.VLLM]: 'https://docs.vllm.ai/en/latest/',
  [LLMFactory.TokenPony]: 'https://docs.tokenpony.cn/#/',
};

// Mapping of factories to their supported model types (for non-dynamic providers)
const llmFactoryModelTypeOptions: Partial<Record<LLMFactory, string[]>> = {
  [LLMFactory.HuggingFace]: ['embedding', 'chat', 'rerank'],
  [LLMFactory.LMStudio]: ['chat', 'embedding', 'image2text'],
  [LLMFactory.Xinference]: [
    'chat',
    'embedding',
    'rerank',
    'image2text',
    'speech2text',
    'tts',
  ],
  [LLMFactory.ModelScope]: ['chat'],
  [LLMFactory.GPUStack]: ['chat', 'embedding', 'rerank', 'speech2text', 'tts'],
};

// Props interface for ModelFormContent component
interface ModelFormContentProps {
  isDynamicProvider: boolean;
  modelsLoading: boolean;

  allModels: IDynamicModel[];
  selectedProvider: string | null;
  selectedModelType: string;
  defaultBaseUrl?: string;
  setSelectedProvider: (provider: string | null) => void;
  setSelectedModelType: (type: string) => void;
  refetchModels: () => void;
  url: string;
  llmFactory: string;
  hideModal?: () => void;
  loading?: boolean;
  handleOk: (values?: FieldValues) => Promise<void>;
  t: (key: string, options?: any) => string;
  tc: (key: string, options?: any) => string;
  shouldShowNoModels: boolean;
}

// Separate component to use useFormContext inside DynamicForm.Root
const ModelFormContent = ({
  isDynamicProvider,
  modelsLoading,

  allModels,
  selectedProvider,
  selectedModelType,
  defaultBaseUrl,
  setSelectedProvider,
  setSelectedModelType,
  refetchModels,
  url,
  llmFactory,
  hideModal,
  loading,
  handleOk,
  t,
  tc,
  shouldShowNoModels,
}: ModelFormContentProps) => {
  const form = useFormContext();

  // Auto-populate base_url when modal opens (only if not editing)
  useEffect(() => {
    if (isDynamicProvider && defaultBaseUrl && !form.getValues('api_base')) {
      form.setValue('api_base', defaultBaseUrl);
    }
  }, [isDynamicProvider, defaultBaseUrl, form]);

  // Auto-populate max_tokens and model_type when model is selected
  const selectedModel = form.watch('llm_name');

  useEffect(() => {
    if (isDynamicProvider && selectedModel) {
      // Search in allModels since dynamicModels is filtered by category
      const model = allModels.find((m: any) => m.llm_name === selectedModel);
      if (model) {
        // Auto-populate max_tokens
        form.setValue('max_tokens', model.max_tokens);

        // Ensure model_type matches the actual model type
        if (model.model_type !== form.getValues('model_type')) {
          form.setValue('model_type', model.model_type);
        }
      }
    }
  }, [selectedModel, isDynamicProvider, allModels, form]);

  // Watch for model_type changes only - provider_filter is handled by form state
  useEffect(() => {
    const subscription = form.watch((value, { name }) => {
      if (name === 'model_type' && isDynamicProvider) {
        const newModelType = value.model_type;
        if (newModelType !== selectedModelType) {
          setSelectedModelType(newModelType);
          // Reset provider and model selection when category changes
          setSelectedProvider(null);
          form.setValue('provider_filter', '', { shouldValidate: false });
          form.setValue('llm_name', '', { shouldValidate: false });
        }
      }

      if (name === 'provider_filter') {
        const newProvider = value.provider_filter;
        // Update state to filter models
        setSelectedProvider(newProvider || null);
      }

      if (name === 'llm_name' && isDynamicProvider && value.llm_name) {
        // When a model is selected, clear the provider filter state
        // but keep the form field value for display
        // This allows the dropdown to show what was selected
      }
    });

    return () => subscription.unsubscribe();
  }, [
    form,
    isDynamicProvider,
    selectedModelType,
    setSelectedModelType,
    setSelectedProvider,
  ]);

  return (
    <>
      {isDynamicProvider && (
        <div className="flex justify-end mb-4">
          <Button
            variant="outline"
            size="sm"
            onClick={() => refetchModels()}
            disabled={modelsLoading}
          >
            {modelsLoading
              ? tc('refreshing', 'Refreshing...')
              : tc('refresh', 'Refresh')}
          </Button>
        </div>
      )}

      {isDynamicProvider && modelsLoading && (
        <div className="text-sm text-muted-foreground mb-2">
          {tc('loadingModels', 'Loading models')}...
        </div>
      )}

      {isDynamicProvider && shouldShowNoModels && (
        <div className="text-sm text-destructive mb-2">
          {selectedProvider
            ? t('noModelsForProvider', {
                provider: selectedProvider,
              })
            : t('noModelsAvailable')}
        </div>
      )}

      <div className="flex items-center justify-between w-full gap-2 ">
        <a href={url} target="_blank" rel="noreferrer" className="text-sm">
          {t('ollamaLink', { name: llmFactory })}
        </a>
        <div className="flex gap-2">
          <DynamicForm.CancelButton
            handleCancel={() => {
              hideModal?.();
            }}
          />
          <DynamicForm.SavingButton
            submitLoading={loading || false}
            buttonText={tc('ok')}
            submitFunc={handleOk}
          />
        </div>
      </div>
    </>
  );
};

const OllamaModal = ({
  visible,
  hideModal,
  onOk,
  loading,
  llmFactory,
  editMode = false,
  initialValues,
  isDynamicProvider: isDynamicProviderProp,
}: IModalProps<Partial<IAddLlmRequestBody> & { provider_order?: string }> & {
  llmFactory: string;
  editMode?: boolean;
  isDynamicProvider?: boolean;
}) => {
  const { t } = useTranslate('setting');
  const { t: tc } = useCommonTranslation();
  const { buildModelTypeOptions } = useBuildModelTypeOptions();

  // State for dynamic model support
  const [selectedProvider, setSelectedProvider] = useState<string | null>(null);
  const [selectedModelType, setSelectedModelType] = useState<string>('chat');

  // Use prop to determine if provider is dynamic, falling back to internal list
  // The parent component determines this via isDynamicProvider() utility
  const canBeDynamic =
    isDynamicProviderProp ??
    [
      LLMFactory.OpenRouter,
      LLMFactory.Ollama,
      LLMFactory.LocalAI,
      LLMFactory.Xinference,
    ].includes(llmFactory as LLMFactory);

  // Fetch dynamic models if provider supports it
  // Only queries backend when canBeDynamic is true and modal is visible
  const {
    data: allModels,
    dataByCategory,
    supportedCategories,
    defaultBaseUrl,
    loading: modelsLoading,
    refetch: refetchModels,
  } = useFetchFactoryModels(
    llmFactory,
    selectedModelType,
    canBeDynamic && visible,
  );

  // Provide a sensible default base URL for OpenRouter while loading
  const effectiveDefaultBaseUrl = useMemo(() => {
    if (defaultBaseUrl) return defaultBaseUrl;
    if (llmFactory === LLMFactory.OpenRouter) {
      return 'https://openrouter.ai/api/v1';
    }
    return null;
  }, [defaultBaseUrl, llmFactory]);

  // Get models for the currently selected category
  const dynamicModels = useMemo(() => {
    if (!selectedModelType || !dataByCategory) return allModels;
    return dataByCategory[selectedModelType] || [];
  }, [selectedModelType, dataByCategory, allModels]);

  // Debug logging for prop reception and category changes

  // Determine if provider should use dynamic UI
  // Honor parent prop if provided; otherwise, enable dynamic UI during loading
  // and once supported categories arrive from backend.
  const isDynamicProvider = useMemo(() => {
    // For OpenRouter, always use dynamic UI and fetch models
    if (llmFactory === LLMFactory.OpenRouter) {
      return true;
    }
    if (typeof isDynamicProviderProp === 'boolean') {
      return (
        isDynamicProviderProp &&
        (supportedCategories.length > 0 || modelsLoading)
      );
    }
    return canBeDynamic && (supportedCategories.length > 0 || modelsLoading);
  }, [
    isDynamicProviderProp,
    canBeDynamic,
    supportedCategories,
    modelsLoading,
    llmFactory,
  ]);

  // Extract unique providers from models for the CURRENT CATEGORY
  // This implements cascading filtering: category → providers → models
  const availableProviders = useMemo(() => {
    if (!isDynamicProvider) {
      return [];
    }

    // If we have models, extract providers from them
    if (dynamicModels.length > 0) {
      const providers = Array.from(
        new Set(dynamicModels.map((m) => m.provider)),
      ).sort();
      return providers;
    }

    // While loading, show a placeholder for known factories
    // This ensures the provider filter dropdown is visible even during initial load
    if (modelsLoading && llmFactory === LLMFactory.OpenRouter) {
      return ['groq', 'fireworks', 'together', 'openai']; // Common OpenRouter providers
    }

    return [];
  }, [dynamicModels, isDynamicProvider, modelsLoading, llmFactory]);

  // Filter models by selected provider
  const filteredModels = useMemo(() => {
    if (!isDynamicProvider) return [];

    let models = dynamicModels;

    // Filter by selected provider if one is selected
    if (selectedProvider) {
      models = models.filter((m) => m.provider === selectedProvider);
    }

    return models;
  }, [dynamicModels, selectedProvider, isDynamicProvider]);

  // Fallback model type options for non-dynamic providers
  const modelTypeOptions = useMemo(() => {
    if (isDynamicProvider) {
      // Use supported categories from backend if available; otherwise fallback
      if (supportedCategories && supportedCategories.length > 0) {
        return buildModelTypeOptions(supportedCategories);
      }
      // Fallback categories while loading for OpenRouter
      if (llmFactory === LLMFactory.OpenRouter) {
        return buildModelTypeOptions(['chat', 'embedding', 'image2text']);
      }
      return buildModelTypeOptions([
        'chat',
        'embedding',
        'rerank',
        'image2text',
      ]);
    }

    // Use predefined options for static providers
    const factoryOptions = llmFactoryModelTypeOptions[llmFactory as LLMFactory];
    if (factoryOptions) {
      return buildModelTypeOptions(factoryOptions);
    }

    // Default fallback
    return buildModelTypeOptions(['chat', 'embedding', 'rerank', 'image2text']);
  }, [
    isDynamicProvider,
    supportedCategories,
    buildModelTypeOptions,
    llmFactory,
  ]);

  const url =
    llmFactoryToUrlMap[llmFactory as LLMFactory] ||
    'https://github.com/infiniflow/ragflow/blob/main/docs/guides/models/deploy_local_llm.mdx';

  // Use refs to access latest values without triggering fields rebuild
  // Update refs synchronously during render (before useMemo runs)
  const availableProvidersRef = useRef(availableProviders);
  const filteredModelsRef = useRef(filteredModels);

  availableProvidersRef.current = availableProviders;
  filteredModelsRef.current = filteredModels;

  // While loading, show placeholder models so model select appears enabled
  const displayModels = useMemo(() => {
    if (filteredModels.length > 0) return filteredModels;
    if (modelsLoading && isDynamicProvider) {
      return [
        {
          llm_name: '',
          name: 'Loading...',
          max_tokens: 0,
          model_type: selectedModelType,
          provider: '',
        } as IDynamicModel,
      ];
    }
    return filteredModels;
  }, [filteredModels, modelsLoading, isDynamicProvider, selectedModelType]);

  const fields = useMemo<FormFieldConfig[]>(() => {
    const baseFields: FormFieldConfig[] = [
      // Model Type selection (always first)
      {
        name: 'model_type',
        label: t('modelType'),
        type: FormFieldType.Select,
        required: true,
        options: modelTypeOptions,
        validation: {
          message: t('modelTypeMessage'),
        },
      },

      // Provider selection (only for dynamic providers with available providers or while loading)
      ...(isDynamicProvider && (availableProviders.length > 0 || modelsLoading)
        ? [
            {
              name: 'provider_filter',
              label: t('availableModels', 'Available Models'),
              type: FormFieldType.Select,
              required: false,
              placeholder: modelsLoading
                ? t('loading', 'Loading...')
                : t('selectModel', 'Select model (optional)'),
              options: availableProviders.map((p) => ({
                value: p,
                label: p.charAt(0).toUpperCase() + p.slice(1),
              })),
              disabled: modelsLoading,
            } as FormFieldConfig,
          ]
        : []),

      // Model selection (dropdown for dynamic, text input for static)
      isDynamicProvider
        ? {
            name: 'llm_name',
            label: t('model', 'Model'),
            type: FormFieldType.Select,
            required: true,
            placeholder: modelsLoading
              ? tc('loading', 'Loading...')
              : t('selectModel', 'Select model'),
            options: displayModels.map((model) => {
              const tokensDisplay =
                typeof model.max_tokens === 'number' &&
                isFinite(model.max_tokens) &&
                model.max_tokens > 0
                  ? `${Math.round(model.max_tokens / 1000)}K`
                  : '—';
              return {
                value: model.llm_name,
                label: `${model.name} (${tokensDisplay})`,
              };
            }),
            disabled:
              modelsLoading || (filteredModels.length === 0 && !modelsLoading),
          }
        : {
            name: 'llm_name',
            label: t(llmFactory === 'Xinference' ? 'modelUid' : 'modelName'),
            type: FormFieldType.Text,
            required: true,
            placeholder: t('modelNameMessage'),
            validation: {
              message: t('modelNameMessage'),
            },
          },

      // Base URL (auto-populated for dynamic with default, required for static)
      {
        name: 'api_base',
        label: t('addLlmBaseUrl'),
        type: FormFieldType.Text,
        required: !isDynamicProvider || !effectiveDefaultBaseUrl,
        placeholder: effectiveDefaultBaseUrl || t('baseUrlNameMessage'),
        validation: {
          message: t('baseUrlNameMessage'),
        },
        tooltip:
          isDynamicProvider && effectiveDefaultBaseUrl
            ? t(
                'defaultBaseUrlTooltip',
                'Default base URL is pre-filled, you can override it',
              )
            : undefined,
      },

      // API Key
      {
        name: 'api_key',
        label: t('apiKey'),
        type: FormFieldType.Text,
        required: false,
        placeholder: isDynamicProvider
          ? t(
              'apiKeyOptional',
              'API Key (optional - reuses existing key if empty)',
            )
          : t('apiKeyMessage'),
      },

      // Max Tokens field (only visible for non-dynamic providers)
      // For dynamic providers: hidden but still in form state (auto-populated)
      ...(isDynamicProvider
        ? []
        : [
            {
              name: 'max_tokens',
              label: t('maxTokens'),
              type: FormFieldType.Number,
              required: true,
              placeholder: t('maxTokensTip'),
              validation: {
                message: t('maxTokensMessage'),
              },
              customValidate: (value: any) => {
                if (value !== undefined && value !== null && value !== '') {
                  if (typeof value !== 'number') {
                    return t('maxTokensInvalidMessage');
                  }
                  if (value < 0) {
                    return t('maxTokensMinMessage');
                  }
                }
                return true;
              },
            } as FormFieldConfig,
          ]),
    ];

    // Add provider_order field only for OpenRouter (legacy support)
    if (llmFactory === LLMFactory.OpenRouter && !isDynamicProvider) {
      baseFields.push({
        name: 'provider_order',
        label: 'Provider Order',
        type: FormFieldType.Text,
        required: false,
        tooltip: 'Comma-separated provider list, e.g. Groq,Fireworks',
        placeholder: 'Groq,Fireworks',
      });
    }

    // Vision switch only for non-dynamic providers with chat model type
    if (!isDynamicProvider) {
      baseFields.push({
        name: 'vision',
        label: t('vision'),
        type: FormFieldType.Switch,
        required: false,
        dependencies: ['model_type'],
        shouldRender: (formValues: any) => {
          return formValues?.model_type === 'chat';
        },
      });
    }

    return baseFields;
  }, [
    llmFactory,
    t,
    tc,
    isDynamicProvider,
    modelsLoading,
    effectiveDefaultBaseUrl,
    modelTypeOptions,
    availableProviders,
    displayModels,
    filteredModels,
  ]);

  const defaultValues: FieldValues = useMemo(() => {
    if (editMode && initialValues) {
      return {
        llm_name: initialValues.llm_name || '',
        model_type: initialValues.model_type || 'chat',
        api_base: initialValues.api_base || defaultBaseUrl || '',
        max_tokens: initialValues.max_tokens || 8192,
        api_key: '',
        vision: initialValues.model_type === 'image2text',
        provider_order: initialValues.provider_order || '',
      };
    }
    return {
      // Always default to 'chat' as the most common use case
      // This matches the selectedModelType state initialization
      model_type: 'chat',
      api_base: defaultBaseUrl || '',
      vision: false,
      provider_filter: '', // Initialize provider filter to ensure dropdown value persists
      max_tokens: 8192, // Default for dynamic providers (will be auto-populated when model selected)
    };
  }, [editMode, initialValues, defaultBaseUrl]);

  const handleOk = async (values?: FieldValues) => {
    if (!values) return;

    // For non-dynamic providers, convert vision=true to image2text model type
    const modelType =
      !isDynamicProvider && values.model_type === 'chat' && values.vision
        ? 'image2text'
        : values.model_type;

    // For dynamic providers, max_tokens might not be in form values (hidden field)
    // Retrieve it from form state or use a sensible default
    const maxTokens = values.max_tokens ?? 8192;

    const data: IAddLlmRequestBody & { provider_order?: string } = {
      llm_factory: llmFactory,
      llm_name: values.llm_name as string,
      model_type: modelType,
      api_base: values.api_base as string,
      api_key: values.api_key as string,
      max_tokens: maxTokens as number,
    };

    // For OpenRouter, always send provider_order (empty string for dynamic mode)
    // This ensures the backend can properly handle API key JSON format
    if (llmFactory === LLMFactory.OpenRouter) {
      data.provider_order = values.provider_order || '';
    } else if (values.provider_order) {
      // For other factories, only add if it exists
      data.provider_order = values.provider_order as string;
    }

    await onOk?.(data);
  };

  return (
    <Modal
      title={<LLMHeader name={llmFactory} />}
      open={visible || false}
      onOpenChange={(open) => !open && hideModal?.()}
      maskClosable={false}
      footer={<></>}
      footerClassName="py-1"
    >
      <DynamicForm.Root
        key={`${visible}-${llmFactory}`}
        fields={fields}
        onSubmit={() => {}}
        defaultValues={defaultValues}
        labelClassName="font-normal"
      >
        <ModelFormContent
          isDynamicProvider={isDynamicProvider}
          modelsLoading={modelsLoading}
          allModels={allModels}
          selectedProvider={selectedProvider}
          selectedModelType={selectedModelType}
          defaultBaseUrl={effectiveDefaultBaseUrl ?? undefined}
          setSelectedProvider={setSelectedProvider}
          setSelectedModelType={setSelectedModelType}
          refetchModels={refetchModels}
          url={url}
          llmFactory={llmFactory}
          hideModal={hideModal}
          loading={loading}
          handleOk={handleOk}
          t={t}
          tc={tc}
          shouldShowNoModels={
            isDynamicProvider &&
            !modelsLoading &&
            filteredModels.length === 0 &&
            supportedCategories.length > 0
          }
        />
      </DynamicForm.Root>
    </Modal>
  );
};

export default OllamaModal;
