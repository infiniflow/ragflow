import { useTranslate } from '@/hooks/commonHooks';
import { IModalProps } from '@/interfaces/common';
import { IAddLlmRequestBody } from '@/interfaces/request/llm';
import { Form, Input, Modal, Select, Switch } from 'antd';
import omit from 'lodash/omit';

type FieldType = IAddLlmRequestBody & { vision: boolean };

const { Option } = Select;

const OllamaModal = ({
  visible,
  hideModal,
  onOk,
  loading,
}: IModalProps<IAddLlmRequestBody>) => {
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
      llm_factory: 'Ollama',
    };
    console.info(data);

    onOk?.(data);
  };

  return (
    <Modal
      title={t('addLlmTitle')}
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
          rules={[{ required: true, message: t('modelNameMessage') }]}
        >
          <Input placeholder={t('modelNameMessage')} />
        </Form.Item>
        <Form.Item<FieldType>
          label={t('addLlmBaseUrl')}
          name="api_base"
          rules={[{ required: true, message: t('baseUrlNameMessage') }]}
        >
          <Input placeholder={t('baseUrlNameMessage')} />
        </Form.Item>
        <Form.Item noStyle dependencies={['model_type']}>
          {({ getFieldValue }) =>
            getFieldValue('model_type') === 'chat' && (
              <Form.Item
                label={t('vision')}
                valuePropName="checked"
                name={'vision'}
              >
                <Switch />
              </Form.Item>
            )
          }
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default OllamaModal;
