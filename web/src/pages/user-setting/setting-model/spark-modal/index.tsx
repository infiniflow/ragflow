import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { IAddLlmRequestBody } from '@/interfaces/request/llm';
import { Form, Input, Modal, Select } from 'antd';
import omit from 'lodash/omit';

type FieldType = IAddLlmRequestBody & {
  vision: boolean;
  spark_api_password: string;
  spark_app_id: string;
  spark_api_secret: string;
  spark_api_key: string;
};

const { Option } = Select;

const SparkModal = ({
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
    const modelType =
      values.model_type === 'chat' && values.vision
        ? 'image2text'
        : values.model_type;

    const data = {
      ...omit(values, ['vision']),
      model_type: modelType,
      llm_factory: llmFactory,
    };
    console.info(data);

    onOk?.(data);
  };

  return (
    <Modal
      title={t('addLlmTitle', { name: llmFactory })}
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
      okButtonProps={{ loading }}
      confirmLoading={loading}
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
            <Option value="tts">tts</Option>
          </Select>
        </Form.Item>
        <Form.Item<FieldType>
          label={t('modelName')}
          name="llm_name"
          rules={[{ required: true, message: t('SparkModelNameMessage') }]}
        >
          <Input placeholder={t('modelNameMessage')} />
        </Form.Item>
        <Form.Item noStyle dependencies={['model_type']}>
          {({ getFieldValue }) =>
            getFieldValue('model_type') === 'chat' && (
                <Form.Item<FieldType>
                  label={t('addSparkAPIPassword')}
                  name="spark_api_password"
                  rules={[{ required: true, message: t('SparkAPIPasswordMessage') }]}
                >
                  <Input placeholder={t('SparkAPIPasswordMessage')} />
                </Form.Item>
            )
          }
        </Form.Item>
        <Form.Item noStyle dependencies={['model_type']}>
          {({ getFieldValue }) =>
            getFieldValue('model_type') === 'tts' && (
                <Form.Item<FieldType>
                  label={t('addSparkAPPID')}
                  name="spark_app_id"
                  rules={[{ required: true, message: t('SparkAPPIDMessage') }]}
                >
                  <Input placeholder={t('SparkAPPIDMessage')} />
                </Form.Item>
            )
          }
        </Form.Item>
        <Form.Item noStyle dependencies={['model_type']}>
          {({ getFieldValue }) =>
            getFieldValue('model_type') === 'tts' && (
                <Form.Item<FieldType>
                  label={t('addSparkAPISecret')}
                  name="spark_api_secret"
                  rules={[{ required: true, message: t('SparkAPISecretMessage') }]}
                >
                  <Input placeholder={t('SparkAPISecretMessage')} />
                </Form.Item>
            )
          }
        </Form.Item>
        <Form.Item noStyle dependencies={['model_type']}>
          {({ getFieldValue }) =>
            getFieldValue('model_type') === 'tts' && (
                <Form.Item<FieldType>
                  label={t('addSparkAPIKey')}
                  name="spark_api_key"
                  rules={[{ required: true, message: t('SparkAPIKeyMessage') }]}
                >
                  <Input placeholder={t('SparkAPIKeyMessage')} />
                </Form.Item>
            )
          }
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default SparkModal;
