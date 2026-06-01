import {
  DynamicForm,
  DynamicFormRef,
  FormFieldConfig,
  FormFieldType,
} from '@/components/dynamic-form';
import { Modal } from '@/components/ui/modal/modal';
import { LLMFactory } from '@/constants/llm';
import { useCommonTranslation, useTranslate } from '@/hooks/common-hooks';
import { useBuildModelTypeOptions } from '@/hooks/logic-hooks/use-build-options';
import { IModalProps } from '@/interfaces/common';
import { IAddProviderInstanceRequestBody } from '@/interfaces/request/llm';
import {
  useFetchInstanceNameSet,
  useHideWhenInstanceExists,
  VerifyResult,
} from '@/pages/user-setting/setting-model/hooks';
import { memo, useCallback, useMemo, useRef } from 'react';
import { FieldValues } from 'react-hook-form';
import { LLMHeader } from '../../components/llm-header';
import VerifyButton from '../../modal/verify-button';

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
  [LLMFactory.RAGcon]: 'https://www.ragcon.ai/erste-schritte-mit-ragflow/',
  [LLMFactory.TogetherAI]: 'https://docs.together.ai/docs/deployment-options',
  [LLMFactory.Replicate]: 'https://replicate.com/docs/topics/deployments',
  [LLMFactory.OpenRouter]: 'https://openrouter.ai/docs',
  [LLMFactory.HuggingFace]:
    'https://huggingface.co/docs/text-embeddings-inference/quick_tour',
  [LLMFactory.GPUStack]: 'https://docs.gpustack.ai/latest/quickstart',
  [LLMFactory.VLLM]: 'https://docs.vllm.ai/en/latest/',
  [LLMFactory.TokenPony]: 'https://docs.tokenpony.cn/#/',
};

function buildModelTypesWithVision(
  modelType: string[] | string,
  vision = false,
): string[] {
  const modelTypeArray = Array.isArray(modelType) ? modelType : [modelType];

  if (modelTypeArray.includes('chat') && vision) {
    return [...modelTypeArray, 'image2text'];
  }

  return modelTypeArray;
}

const OllamaModal = ({
  visible,
  hideModal,
  onOk,
  onVerify,
  loading,
  llmFactory,
  editMode = false,
  initialValues,
}: IModalProps<
  Partial<IAddProviderInstanceRequestBody> & { provider_order?: string }
> & {
  llmFactory: string;
  editMode?: boolean;
  onVerify?: (
    postBody: any,
  ) => Promise<boolean | void | VerifyResult | undefined>;
}) => {
  const { t } = useTranslate('setting');
  const { t: tc } = useCommonTranslation();
  const { buildModelTypeOptions } = useBuildModelTypeOptions();
  const formRef = useRef<DynamicFormRef>(null);
  const { instanceNameSet } = useFetchInstanceNameSet(llmFactory);

  const hideWhenInstanceExists = useHideWhenInstanceExists(instanceNameSet);

  const optionsMap: Partial<
    Record<LLMFactory, { label: string; value: string }[]>
  > & {
    Default: { label: string; value: string }[];
  } = {
    [LLMFactory.HuggingFace]: buildModelTypeOptions([
      'embedding',
      'chat',
      'rerank',
    ]),
    [LLMFactory.LMStudio]: buildModelTypeOptions([
      'chat',
      'embedding',
      'image2text',
    ]),
    [LLMFactory.Xinference]: buildModelTypeOptions([
      'chat',
      'embedding',
      'rerank',
      'image2text',
      'speech2text',
      'tts',
    ]),
    [LLMFactory.RAGcon]: buildModelTypeOptions([
      'chat',
      'embedding',
      'rerank',
      'image2text',
      'speech2text',
      'tts',
    ]),
    [LLMFactory.ModelScope]: buildModelTypeOptions(['chat']),
    [LLMFactory.GPUStack]: buildModelTypeOptions([
      'chat',
      'embedding',
      'rerank',
      'speech2text',
      'tts',
    ]),
    [LLMFactory.OpenRouter]: buildModelTypeOptions(['chat', 'image2text']),
    Default: buildModelTypeOptions([
      'chat',
      'embedding',
      'rerank',
      'image2text',
    ]),
  };

  const url =
    llmFactoryToUrlMap[llmFactory as LLMFactory] ||
    'https://github.com/infiniflow/ragflow/blob/main/docs/guides/models/deploy_local_llm.mdx';

  const fields = useMemo<FormFieldConfig[]>(() => {
    const getOptions = (factory: string) => {
      return optionsMap[factory as LLMFactory] || optionsMap.Default;
    };
    const defaultToolCallEnabled = initialValues?.is_tools ?? false;

    const baseFields: FormFieldConfig[] = [
      {
        name: 'instance_name',
        label: t('instanceName'),
        type: FormFieldType.Text,
        required: true,
        placeholder: t('instanceNameMessage'),
        tooltip: t('instanceNameTip'),
        validation: {
          message: t('instanceNameMessage'),
        },
      },
      {
        name: 'model_type',
        label: t('modelType'),
        type: FormFieldType.MultiSelect,
        required: true,
        options: getOptions(llmFactory),
      },
      {
        name: 'llm_name',
        label: t(llmFactory === 'Xinference' ? 'modelUid' : 'modelName'),
        type: FormFieldType.Text,
        required: true,
        placeholder: t('modelNameMessage'),
        validation: {
          message: t('modelNameMessage'),
        },
      },
      {
        name: 'api_base',
        label: t('addLlmBaseUrl'),
        type: FormFieldType.Text,
        required: true,
        placeholder: t('baseUrlNameMessage'),
        validation: {
          message: t('baseUrlNameMessage'),
        },
        shouldRender: hideWhenInstanceExists,
      },
      {
        name: 'api_key',
        label: t('apiKey'),
        type: FormFieldType.Text,
        required: false,
        placeholder: t('apiKeyMessage'),
        shouldRender: hideWhenInstanceExists,
      },
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
      },
    ];

    baseFields.push({
      name: 'is_tools',
      label: t('enableToolCall'),
      type: FormFieldType.Switch,
      required: false,
      dependencies: ['model_type'],
      shouldRender: (formValues: any) => {
        const modelType = formValues?.model_type;
        if (Array.isArray(modelType)) {
          return modelType.includes('chat') || modelType.includes('image2text');
        }
        return modelType === 'chat' || modelType === 'image2text';
      },
      tooltip: t('enableToolCallTip'),
      defaultValue: defaultToolCallEnabled,
    });

    // Add provider_order field only for OpenRouter
    if (llmFactory === 'OpenRouter') {
      baseFields.push({
        name: 'provider_order',
        label: 'Provider Order',
        type: FormFieldType.Text,
        required: false,
        tooltip: 'Comma-separated provider list, e.g. Groq,Fireworks',
        placeholder: 'Groq,Fireworks',
      });
    }

    // Add vision switch (conditional on model_type === 'chat')
    baseFields.push({
      name: 'vision',
      label: t('vision'),
      type: FormFieldType.Switch,
      required: false,
      dependencies: ['model_type'],
      shouldRender: (formValues: any) => {
        const modelType = formValues?.model_type;
        if (Array.isArray(modelType)) {
          return modelType.includes('chat');
        }
        return modelType === 'chat';
      },
    });

    return baseFields;
  }, [llmFactory, t, hideWhenInstanceExists, initialValues?.is_tools]);

  const defaultValues: FieldValues = useMemo(() => {
    if (editMode && initialValues) {
      return {
        instance_name: initialValues.instance_name || '',
        llm_name: initialValues.llm_name || '',
        model_type: initialValues.model_type
          ? initialValues.model_type
          : ['chat'],
        api_base: initialValues.api_base || '',
        max_tokens: initialValues.max_tokens || 8192,
        api_key: '',
        vision: initialValues.model_type === 'image2text',
        provider_order: initialValues.provider_order || '',
        is_tools: initialValues.is_tools || false,
      };
    }
    return {
      instance_name: '',
      model_type: [
        llmFactory === LLMFactory.Ollama || llmFactory === LLMFactory.VLLM
          ? 'chat'
          : llmFactory in optionsMap
            ? optionsMap[llmFactory as LLMFactory]?.at(0)?.value
            : 'embedding',
      ],
      vision: false,
      is_tools: false,
      max_tokens: 8192,
    };
  }, [editMode, initialValues, llmFactory]);

  const handleOk = async (values?: FieldValues) => {
    if (!values) return;

    const modelTypeArray: string[] = Array.isArray(values.model_type)
      ? values.model_type
      : [values.model_type];
    const supportsToolCall =
      modelTypeArray.includes('chat') || modelTypeArray.includes('image2text');

    const data: IAddProviderInstanceRequestBody & { provider_order?: string } =
      {
        instance_name: values.instance_name as string,
        llm_factory: llmFactory,
        llm_name: values.llm_name as string,
        model_type: buildModelTypesWithVision(values.model_type, values.vision),
        api_base: values.api_base as string,
        api_key: values.api_key as string,
        max_tokens: values.max_tokens as number,
      };
    if (supportsToolCall) {
      data.is_tools = Boolean(values.is_tools);
    }

    // Add provider_order only if it exists (for OpenRouter)
    if (values.provider_order) {
      data.provider_order = values.provider_order as string;
    }

    await onOk?.(data);
  };

  const verifyParamsFunc = useCallback(() => {
    const values = formRef.current?.getValues();
    return {
      llm_factory: llmFactory,
      model_type: buildModelTypesWithVision(values.model_type, values.vision),
    };
  }, [llmFactory]);

  const handleVerify = useCallback(
    async (params: any) => {
      const verifyParams = verifyParamsFunc();
      const res = await onVerify?.({ ...params, ...verifyParams });
      return (res || { isValid: null, logs: '' }) as VerifyResult;
    },
    [verifyParamsFunc, onVerify],
  );

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
        ref={formRef}
        onSubmit={() => {}}
        defaultValues={defaultValues}
        labelClassName="font-normal"
      >
        {onVerify && (
          <VerifyButton onVerify={handleVerify} isAbsolute={false} />
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
              submitFunc={(values: FieldValues) => {
                handleOk(values);
              }}
            />
          </div>
        </div>
      </DynamicForm.Root>
    </Modal>
  );
};

export default memo(OllamaModal);
