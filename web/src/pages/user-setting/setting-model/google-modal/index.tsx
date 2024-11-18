import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { IAddLlmRequestBody } from '@/interfaces/request/llm';
import { Form, Input, Modal, Select, InputNumber } from 'antd';

type FieldType = IAddLlmRequestBody & {
  google_project_id: string;
  google_region: string;
  google_service_account_key: string;
};

const { Option } = Select;

const GoogleModal = ({
  visible,
  hideModal,
  onOk,
  loading,
  llmFactory,
}: IModalProps<IAddLlmRequestBody> & { llmFactory: string }) => {
  const [form] = Form.useForm<FieldType>();

  const { t } = useTranslate('setting');
  const handleOk = async () => {
    const values = await form.validateFields();

    const data = {
      ...values,
      llm_factory: llmFactory,
      max_tokens:values.max_tokens,
    };

    onOk?.(data);
  };

  return (
    <Modal
      title={t('addLlmTitle', { name: llmFactory })}
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
      okButtonProps={{ loading }}
    >
      <Form
        name="basic"
        style={{ maxWidth: 600 }}
        autoComplete="off"
        layout={'vertical'}
        form={form}
      >
        <Form.Item<FieldType>
          label={t('modelType')}
          name="model_type"
          initialValue={'chat'}
          rules={[{ required: true, message: t('modelTypeMessage') }]}
        >
          <Select placeholder={t('modelTypeMessage')}>
            <Option value="chat">chat</Option>
          </Select>
        </Form.Item>
        <Form.Item<FieldType>
          label={t('modelID')}
          name="llm_name"
          rules={[{ required: true, message: t('GoogleModelIDMessage') }]}
        >
          <Input placeholder={t('GoogleModelIDMessage')} />
        </Form.Item>
        <Form.Item<FieldType>
          label={t('addGoogleProjectID')}
          name="google_project_id"
          rules={[{ required: true, message: t('GoogleProjectIDMessage') }]}
        >
          <Input placeholder={t('GoogleProjectIDMessage')} />
        </Form.Item>
        <Form.Item<FieldType>
          label={t('addGoogleRegion')}
          name="google_region"
          rules={[{ required: true, message: t('GoogleRegionMessage') }]}
        >
          <Input placeholder={t('GoogleRegionMessage')} />
        </Form.Item>
        <Form.Item<FieldType>
          label={t('addGoogleServiceAccountKey')}
          name="google_service_account_key"
          rules={[
            { required: true, message: t('GoogleServiceAccountKeyMessage') },
          ]}
        >
          <Input placeholder={t('GoogleServiceAccountKeyMessage')} />
        </Form.Item>
        <Form.Item<FieldType>
          label={t('maxTokens')}
          name="max_tokens"
          rules={[
            { required: true, message: t('maxTokensMessage') },
            {
              type: 'number',
              message: t('maxTokensInvalidMessage'),
            },
            ({ getFieldValue }) => ({
              validator(_, value) {
                if (value < 0) {
                  return Promise.reject(new Error(t('maxTokensMinMessage')));
                }
                return Promise.resolve();
              },
            }),
          ]}
        >
          <InputNumber
            placeholder={t('maxTokensTip')}
            style={{ width: '100%' }}
          />
        </Form.Item>

      </Form>
    </Modal>
  );
};

export default GoogleModal;
