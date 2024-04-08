import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { useTranslate } from '@/hooks/commonHooks';
import { Form, Input, Modal } from 'antd';
import { useEffect } from 'react';

interface IProps extends Omit<IModalManagerChildrenProps, 'showModal'> {
  loading: boolean;
  initialValue: string;
  llmFactory: string;
  onOk: (name: string, baseUrl: string) => void;
  showModal?(): void;
}

type FieldType = {
  api_key?: string;
  base_url?: string;
};

const ApiKeyModal = ({
  visible,
  hideModal,
  llmFactory,
  loading,
  initialValue,
  onOk,
}: IProps) => {
  const [form] = Form.useForm();
  const { t } = useTranslate('setting');

  const handleOk = async () => {
    const ret = await form.validateFields();

    return onOk(ret.api_key, ret.base_url);
  };

  const handleCancel = () => {
    hideModal();
  };

  const onFinish = (values: any) => {
    console.log('Success:', values);
  };

  const onFinishFailed = (errorInfo: any) => {
    console.log('Failed:', errorInfo);
  };

  useEffect(() => {
    if (visible) {
      form.setFieldValue('api_key', initialValue);
    }
  }, [initialValue, form, visible]);

  return (
    <Modal
      title={t('modify')}
      open={visible}
      onOk={handleOk}
      onCancel={handleCancel}
      okButtonProps={{ loading }}
      confirmLoading={loading}
    >
      <Form
        name="basic"
        labelCol={{ span: 6 }}
        wrapperCol={{ span: 18 }}
        style={{ maxWidth: 600 }}
        onFinish={onFinish}
        onFinishFailed={onFinishFailed}
        autoComplete="off"
        form={form}
      >
        <Form.Item<FieldType>
          label={t('apiKey')}
          name="api_key"
          tooltip={t('apiKeyTip')}
          rules={[{ required: true, message: t('apiKeyMessage') }]}
        >
          <Input />
        </Form.Item>
        {llmFactory === 'OpenAI' && (
          <Form.Item<FieldType>
            label={t('baseUrl')}
            name="base_url"
            tooltip={t('baseUrlTip')}
          >
            <Input placeholder="https://api.openai.com/v1" />
          </Form.Item>
        )}
      </Form>
    </Modal>
  );
};

export default ApiKeyModal;
