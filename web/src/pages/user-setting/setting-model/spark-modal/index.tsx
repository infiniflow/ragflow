import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { IAddLlmRequestBody } from '@/interfaces/request/llm';
import { Form, Input, Modal, Select } from 'antd';
import omit from 'lodash/omit';

type FieldType = IAddLlmRequestBody & {
  vision: boolean;
  spark_api_password: string;
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
          </Select>
        </Form.Item>
        <Form.Item<FieldType>
          label={t('modelName')}
          name="llm_name"
          initialValue={'Spark-Max'}
          rules={[{ required: true, message: t('SparkModelNameMessage') }]}
        >
          <Select placeholder={t('modelTypeMessage')}>
            <Option value="Spark-Max">Spark-Max</Option>
            <Option value="Spark-Lite">Spark-Lite</Option>
            <Option value="Spark-Pro">Spark-Pro</Option>
            <Option value="Spark-Pro-128K">Spark-Pro-128K</Option>
            <Option value="Spark-4.0-Ultra">Spark-4.0-Ultra</Option>
          </Select>
        </Form.Item>
        <Form.Item<FieldType>
          label={t('addSparkAPIPassword')}
          name="spark_api_password"
          rules={[{ required: true, message: t('SparkAPIPasswordMessage') }]}
        >
          <Input placeholder={t('SparkAPIPasswordMessage')} />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default SparkModal;
