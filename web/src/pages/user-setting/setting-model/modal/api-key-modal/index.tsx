import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Form } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Modal } from '@/components/ui/modal/modal';
import { LLMFactory } from '@/constants/llm';
import { useTranslate } from '@/hooks/common-hooks';
import { KeyboardEventHandler, useCallback, useEffect } from 'react';
import { useForm } from 'react-hook-form';
import { ApiKeyPostBody } from '../../../interface';
import { LLMHeader } from '../../components/llm-header';
import { VerifyResult } from '../../hooks';
import VerifyButton from '../verify-button';

interface IProps extends Omit<IModalManagerChildrenProps, 'showModal'> {
  loading: boolean;
  initialValue: string;
  llmFactory: string;
  editMode?: boolean;
  onOk: (postBody: ApiKeyPostBody) => void;
  onVerify: (
    postBody: any,
  ) => Promise<boolean | void | VerifyResult | undefined>;
  showModal?(): void;
}

type FieldType = {
  instance_name?: string;
  api_key?: string;
  base_url?: string;
  group_id?: string;
};

const modelsWithBaseUrl = [
  LLMFactory.OpenAI,
  LLMFactory.AzureOpenAI,
  LLMFactory.TongYiQianWen,
  LLMFactory.MiniMax,
  LLMFactory.SILICONFLOW,
];

const ApiKeyModal = ({
  visible,
  hideModal,
  llmFactory,
  loading,
  initialValue,
  // editMode = false,
  onOk,
  onVerify,
}: IProps) => {
  const form = useForm<FieldType>();
  const { t } = useTranslate('setting');

  const handleOk = useCallback(async () => {
    await form.handleSubmit((values) => onOk(values))();
  }, [form, onOk]);

  const handleKeyDown: KeyboardEventHandler<HTMLInputElement> = useCallback(
    async (e) => {
      if (e.key === 'Enter') {
        await handleOk();
      }
    },
    [handleOk],
  );

  useEffect(() => {
    if (visible) {
      form.setValue('api_key', initialValue);
    }
  }, [initialValue, form, visible]);

  return (
    <Modal
      title={<LLMHeader name={llmFactory} />}
      open={visible}
      onOpenChange={(open) => !open && hideModal()}
      onOk={handleOk}
      onCancel={hideModal}
      confirmLoading={loading}
      okText={t('save')}
      cancelText={t('cancel')}
      className="!w-[600px]"
      testId="apikey-modal"
      okButtonTestId="apikey-save"
    >
      <Form {...form}>
        <div className="space-y-4 py-4">
          <RAGFlowFormItem
            name="instance_name"
            label={t('instanceName')}
            tooltip={t('instanceNameTip')}
            rules={{ required: t('instanceNameMessage') }}
            required
            labelClassName="text-sm font-medium text-text-secondary"
          >
            {(field) => (
              <Input
                {...field}
                placeholder={t('instanceNameMessage')}
                onKeyDown={handleKeyDown}
                className="w-full"
              />
            )}
          </RAGFlowFormItem>

          <RAGFlowFormItem
            name="api_key"
            label={t('apiKey')}
            rules={{ required: t('apiKeyMessage') }}
            required
            labelClassName="text-sm font-medium text-text-secondary"
          >
            {(field) => (
              <Input
                {...field}
                data-testid="apikey-input"
                onKeyDown={handleKeyDown}
                className="w-full"
              />
            )}
          </RAGFlowFormItem>

          {modelsWithBaseUrl.some((x) => x === llmFactory) && (
            <RAGFlowFormItem
              name="base_url"
              label={t('baseUrl')}
              tooltip={
                llmFactory === LLMFactory.MiniMax
                  ? t('minimaxBaseUrlTip')
                  : llmFactory === LLMFactory.TongYiQianWen
                    ? t('tongyiBaseUrlTip')
                    : llmFactory === LLMFactory.SILICONFLOW
                      ? t('siliconBaseUrlTip')
                      : t('baseUrlTip')
              }
              labelClassName="text-sm font-medium text-text-primary"
            >
              {(field) => (
                <Input
                  {...field}
                  placeholder={
                    llmFactory === LLMFactory.TongYiQianWen
                      ? t('tongyiBaseUrlPlaceholder')
                      : llmFactory === LLMFactory.MiniMax
                        ? t('minimaxBaseUrlPlaceholder')
                        : llmFactory === LLMFactory.SILICONFLOW
                          ? 'https://api.siliconflow.cn/v1'
                          : 'https://api.openai.com/v1'
                  }
                  onKeyDown={handleKeyDown}
                  className="w-full"
                />
              )}
            </RAGFlowFormItem>
          )}

          {llmFactory?.toLowerCase() === 'Anthropic'.toLowerCase() && (
            <RAGFlowFormItem
              name="base_url"
              label={t('baseUrl')}
              labelClassName="text-sm font-medium text-text-primary"
            >
              {(field) => (
                <Input
                  {...field}
                  placeholder="https://api.anthropic.com/v1"
                  onKeyDown={handleKeyDown}
                  className="w-full"
                />
              )}
            </RAGFlowFormItem>
          )}

          {llmFactory?.toLowerCase() === 'Minimax'.toLowerCase() && (
            <RAGFlowFormItem
              name="group_id"
              label="Group ID"
              labelClassName="text-sm font-medium text-text-primary"
            >
              {(field) => <Input {...field} className="w-full" />}
            </RAGFlowFormItem>
          )}

          <VerifyButton onVerify={onVerify} />
        </div>
      </Form>
    </Modal>
  );
};

export default ApiKeyModal;
