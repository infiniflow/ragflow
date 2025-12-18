import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { IAddLlmRequestBody } from '@/interfaces/request/llm';
import { Form, Input, InputNumber, Modal, Select, Typography } from 'antd';
import { useMemo } from 'react';
import { LLMHeader } from '../../components/llm-header';
import { BedrockRegionList } from '../../constant';

type FieldType = IAddLlmRequestBody & {
  bedrock_ak: string;
  bedrock_sk: string;
  bedrock_region: string;
};

const { Option } = Select;
const { Text } = Typography;

const BedrockModal = ({
  visible,
  hideModal,
  onOk,
  loading,
  llmFactory,
}: IModalProps<IAddLlmRequestBody> & { llmFactory: string }) => {
  const [form] = Form.useForm<FieldType>();

  const { t } = useTranslate('setting');
  const options = useMemo(
    () => BedrockRegionList.map((x) => ({ value: x, label: t(x) })),
    [t],
  );

  const handleOk = async () => {
    const values = await form.validateFields();

    const data = {
      ...values,
      llm_factory: llmFactory,
      max_tokens: values.max_tokens,
    };

    onOk?.(data);
  };

  return (
    <Modal
      title={
        <div>
          <LLMHeader name={llmFactory} />
          <Text type="secondary" style={{ display: 'block', marginTop: 4 }}>
            {t('bedrockCredentialsHint')}
          </Text>
        </div>
      }
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
            <Option value="embedding">embedding</Option>
          </Select>
        </Form.Item>
        <Form.Item<FieldType>
          label={t('modelName')}
          name="llm_name"
          rules={[{ required: true, message: t('bedrockModelNameMessage') }]}
        >
          <Input placeholder={t('bedrockModelNameMessage')} />
        </Form.Item>
        <Form.Item<FieldType>
          label={t('addBedrockEngineAK')}
          name="bedrock_ak"
          rules={[{ message: t('bedrockAKMessage') }]}
        >
          <Input placeholder={t('bedrockAKMessage')} />
        </Form.Item>
        <Form.Item<FieldType>
          label={t('addBedrockSK')}
          name="bedrock_sk"
          rules={[{ message: t('bedrockSKMessage') }]}
        >
          <Input placeholder={t('bedrockSKMessage')} />
        </Form.Item>
        <Form.Item<FieldType>
          label={t('bedrockRegion')}
          name="bedrock_region"
          rules={[{ required: true, message: t('bedrockRegionMessage') }]}
        >
          <Select
            placeholder={t('bedrockRegionMessage')}
            options={options}
            allowClear
          ></Select>
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
            ({}) => ({
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

export default BedrockModal;
