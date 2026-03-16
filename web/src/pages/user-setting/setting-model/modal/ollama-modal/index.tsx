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
import { IAddLlmRequestBody } from '@/interfaces/request/llm';
import { VerifyResult } from '@/pages/user-setting/setting-model/hooks';
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
  [LLMFactory.TogetherAI]: 'https://docs.together.ai/docs/deployment-options',
  [LLMFactory.Replicate]: 'https://replicate.com/docs/topics/deployments',
  [LLMFactory.OpenRouter]: 'https://openrouter.ai/docs',
  [LLMFactory.HuggingFace]:
    'https://huggingface.co/docs/text-embeddings-inference/quick_tour',
  [LLMFactory.GPUStack]: 'https://docs.gpustack.ai/latest/quickstart',
  [LLMFactory.VLLM]: 'https://docs.vllm.ai/en/latest/',
  [LLMFactory.TokenPony]: 'https://docs.tokenpony.cn/#/',
};

const OllamaModal = ({
  visible,
  hideModal,
  onOk,
  onVerify,
  loading,
  llmFactory,
  editMode = false,
  initialValues,
}: IModalProps<Partial<IAddLlmRequestBody> & { provider_order?: string }> & {
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

    const baseFields: FormFieldConfig[] = [
      {
        name: 'model_type',
        label: t('modelType'),
        type: FormFieldType.Select,
        required: true,
        options: getOptions(llmFactory),
        validation: {
          message: t('modelTypeMessage'),
        },
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
      },
      {
        name: 'api_key',
        label: t('apiKey'),
        type: FormFieldType.Text,
        required: false,
        placeholder: t('apiKeyMessage'),
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
        return formValues?.model_type === 'chat';
      },
    });

    return baseFields;
  }, [llmFactory, t]);

  const defaultValues: FieldValues = useMemo(() => {
    if (editMode && initialValues) {
      return {
        llm_name: initialValues.llm_name || '',
        model_type: initialValues.model_type || 'chat',
        api_base: initialValues.api_base || '',
        max_tokens: initialValues.max_tokens || 8192,
        api_key: '',
        vision: initialValues.model_type === 'image2text',
        provider_order: initialValues.provider_order || '',
      };
    }
    return {
      model_type:
        llmFactory in optionsMap
          ? optionsMap[llmFactory as LLMFactory]?.at(0)?.value
          : 'embedding',
      vision: false,
    };
  }, [editMode, initialValues, llmFactory]);

  const handleOk = async (values?: FieldValues) => {
    if (!values) return;

    const modelType =
      values.model_type === 'chat' && values.vision
        ? 'image2text'
        : values.model_type;

    const data: IAddLlmRequestBody & { provider_order?: string } = {
      llm_factory: llmFactory,
      llm_name: values.llm_name as string,
      model_type: modelType,
      api_base: values.api_base as string,
      api_key: values.api_key as string,
      max_tokens: values.max_tokens as number,
    };

    // Add provider_order only if it exists (for OpenRouter)
    if (values.provider_order) {
      data.provider_order = values.provider_order as string;
    }

    await onOk?.(data);
  };

  const verifyParamsFunc = useCallback(() => {
    const values = formRef.current?.getValues();
    const modelType =
      values.model_type === 'chat' && values.vision
        ? 'image2text'
        : values.model_type;
    return {
      llm_factory: llmFactory,
      model_type: modelType,
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
