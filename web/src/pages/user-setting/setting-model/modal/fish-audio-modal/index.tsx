import {
  DynamicForm,
  FormFieldConfig,
  FormFieldType,
} from '@/components/dynamic-form';
import { Modal } from '@/components/ui/modal/modal';
import { useCommonTranslation, useTranslate } from '@/hooks/common-hooks';
import { useBuildModelTypeOptions } from '@/hooks/logic-hooks/use-build-options';
import { IModalProps } from '@/interfaces/common';
import { IAddProviderInstanceRequestBody } from '@/interfaces/request/llm';
import {
  useFetchInstanceNameSet,
  useHideWhenInstanceExists,
  VerifyResult,
} from '@/pages/user-setting/setting-model/hooks';
import { memo, useCallback, useMemo } from 'react';
import { FieldValues } from 'react-hook-form';
import { LLMHeader } from '../../components/llm-header';
import VerifyButton from '../../modal/verify-button';

const FishAudioModal = ({
  visible,
  hideModal,
  onOk,
  onVerify,
  loading,
  llmFactory,
}: IModalProps<IAddProviderInstanceRequestBody> & {
  llmFactory: string;
  onVerify?: (
    postBody: any,
  ) => Promise<boolean | void | VerifyResult | undefined>;
}) => {
  const { t } = useTranslate('setting');
  const { t: tc } = useCommonTranslation();
  const { buildModelTypeOptions } = useBuildModelTypeOptions();
  const { instanceNameSet } = useFetchInstanceNameSet(llmFactory);

  const hideWhenInstanceExists = useHideWhenInstanceExists(instanceNameSet);

  const fields: FormFieldConfig[] = useMemo(
    () => [
      {
        name: 'instance_name',
        label: t('instanceName'),
        type: FormFieldType.Text,
        required: true,
        placeholder: t('instanceNameMessage'),
        tooltip: t('instanceNameTip'),
        validation: { message: t('instanceNameMessage') },
      },
      {
        name: 'model_type',
        label: t('modelType'),
        type: FormFieldType.MultiSelect,
        required: true,
        options: buildModelTypeOptions(['tts']),
        defaultValue: ['tts'],
      },
      {
        name: 'llm_name',
        label: t('modelName'),
        type: FormFieldType.Text,
        required: true,
        placeholder: t('FishAudioModelNameMessage'),
        validation: { message: t('FishAudioModelNameMessage') },
      },
      {
        name: 'fish_audio_ak',
        label: t('addFishAudioAK'),
        type: FormFieldType.Text,
        required: true,
        placeholder: t('FishAudioAKMessage'),
        validation: { message: t('FishAudioAKMessage') },
        shouldRender: hideWhenInstanceExists,
      },
      {
        name: 'fish_audio_refid',
        label: t('addFishAudioRefID'),
        type: FormFieldType.Text,
        required: true,
        placeholder: t('FishAudioRefIDMessage'),
        validation: { message: t('FishAudioRefIDMessage') },
        shouldRender: hideWhenInstanceExists,
      },
      {
        name: 'max_tokens',
        label: t('maxTokens'),
        type: FormFieldType.Number,
        required: true,
        placeholder: t('maxTokensTip'),
        validation: {
          min: 0,
          message: t('maxTokensInvalidMessage'),
        },
      },
    ],
    [t, buildModelTypeOptions, hideWhenInstanceExists],
  );

  const handleOk = async (values?: FieldValues) => {
    if (!values) return;

    const data: IAddProviderInstanceRequestBody & {
      fish_audio_ak: string;
      fish_audio_refid: string;
    } = {
      instance_name: values.instance_name as string,
      llm_factory: llmFactory,
      llm_name: values.llm_name as string,
      model_type: values.model_type,
      fish_audio_ak: values.fish_audio_ak,
      fish_audio_refid: values.fish_audio_refid,
      max_tokens: values.max_tokens as number,
    };

    await onOk?.(data);
  };

  const handleVerify = useCallback(
    async (params: any) => {
      const res = await onVerify?.({ ...params, llm_factory: llmFactory });
      return (res || { isValid: null, logs: '' }) as VerifyResult;
    },
    [llmFactory, onVerify],
  );

  return (
    <Modal
      title={<LLMHeader name={llmFactory} />}
      open={visible || false}
      onOpenChange={(open) => !open && hideModal?.()}
      maskClosable={false}
      footerClassName="py-1"
      footer={<div className="py-0"></div>}
    >
      <DynamicForm.Root
        fields={fields}
        onSubmit={(data) => console.log(data)}
        defaultValues={{
          instance_name: '',
          model_type: ['tts'],
          max_tokens: 8192,
        }}
        labelClassName="font-normal"
      >
        {onVerify && (
          <VerifyButton onVerify={handleVerify} isAbsolute={false} />
        )}
        <div className="flex items-center justify-between w-full">
          <a
            href="https://fish.audio"
            target="_blank"
            rel="noreferrer"
            className="text-sm text-text-secondary hover:text-primary"
          >
            {t('FishAudioLink')}
          </a>
          <div className="flex gap-2">
            <DynamicForm.CancelButton handleCancel={() => hideModal?.()} />
            <DynamicForm.SavingButton
              submitLoading={loading || false}
              buttonText={tc('ok')}
              submitFunc={(values: FieldValues) => handleOk(values)}
            />
          </div>
        </div>
      </DynamicForm.Root>
    </Modal>
  );
};

export default memo(FishAudioModal);
