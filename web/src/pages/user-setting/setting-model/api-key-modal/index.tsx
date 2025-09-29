import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { LLMFactory } from '@/constants/llm';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Input, Modal } from 'antd';
import { KeyboardEventHandler, useCallback, useEffect } from 'react';
import { ApiKeyPostBody } from '../../interface';

interface IProps extends Omit<IModalManagerChildrenProps, 'showModal'> {
  loading: boolean;
  initialValue: string;
  llmFactory: string;
  editMode?: boolean;
  onOk: (postBody: ApiKeyPostBody) => void;
  showModal?(): void;
}

type FieldType = {
  api_key?: string;
  base_url?: string;
  group_id?: string;
};

const modelsWithBaseUrl = [
  LLMFactory.OpenAI,
  LLMFactory.AzureOpenAI,
  LLMFactory.TongYiQianWen,
];

const ApiKeyModal = ({
  visible,
  hideModal,
  llmFactory,
  loading,
  initialValue,
  editMode = false,
  onOk,
}: IProps) => {
  const [form] = Form.useForm();
  const { t } = useTranslate('setting');

  const handleOk = useCallback(async () => {
    const ret = await form.validateFields();

    return onOk(ret);
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
      form.setFieldValue('api_key', initialValue);
    }
  }, [initialValue, form, visible]);

  return (
    <Modal
      title={editMode ? t('editModel') : t('modify')}
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
      okButtonProps={{ loading }}
      confirmLoading={loading}
    >
      <Form
        name="basic"
        labelCol={{ span: 6 }}
        wrapperCol={{ span: 18 }}
        style={{ maxWidth: 600 }}
        autoComplete="off"
        form={form}
      >
        <Form.Item<FieldType>
          label={t('apiKey')}
          name="api_key"
          tooltip={t('apiKeyTip')}
          rules={[{ required: true, message: t('apiKeyMessage') }]}
        >
          <Input onKeyDown={handleKeyDown} />
        </Form.Item>
        {modelsWithBaseUrl.some((x) => x === llmFactory) && (
          <Form.Item<FieldType>
            label={t('baseUrl')}
            name="base_url"
            tooltip={
              llmFactory === LLMFactory.TongYiQianWen
                ? t('tongyiBaseUrlTip')
                : t('baseUrlTip')
            }
          >
            <Input
              placeholder={
                llmFactory === LLMFactory.TongYiQianWen
                  ? t('tongyiBaseUrlPlaceholder')
                  : 'https://api.openai.com/v1'
              }
              onKeyDown={handleKeyDown}
            />
          </Form.Item>
        )}
        {llmFactory?.toLowerCase() === 'Anthropic'.toLowerCase() && (
          <Form.Item<FieldType>
            label={t('baseUrl')}
            name="base_url"
            tooltip={t('baseUrlTip')}
          >
            <Input
              placeholder="https://api.anthropic.com/v1"
              onKeyDown={handleKeyDown}
            />
          </Form.Item>
        )}
        {llmFactory?.toLowerCase() === 'Minimax'.toLowerCase() && (
          <Form.Item<FieldType> label={'Group ID'} name="group_id">
            <Input />
          </Form.Item>
        )}
      </Form>
    </Modal>
  );
};

export default ApiKeyModal;
