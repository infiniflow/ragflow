import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { Form, Input, Modal } from 'antd';
import { useEffect } from 'react';

interface IProps extends Omit<IModalManagerChildrenProps, 'showModal'> {
  loading: boolean;
  initialValue: string;
  onOk: (name: string) => void;
  showModal?(): void;
}

type FieldType = {
  api_key?: string;
};

const ApiKeyModal = ({
  visible,
  hideModal,
  loading,
  initialValue,
  onOk,
}: IProps) => {
  const [form] = Form.useForm();

  const handleOk = async () => {
    const ret = await form.validateFields();

    return onOk(ret.api_key);
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
    form.setFieldValue('api_key', initialValue);
  }, [initialValue, form]);

  return (
    <Modal
      title="Modify"
      open={visible}
      onOk={handleOk}
      onCancel={handleCancel}
      okButtonProps={{ loading }}
      confirmLoading={loading}
    >
      <Form
        name="basic"
        labelCol={{ span: 4 }}
        wrapperCol={{ span: 20 }}
        style={{ maxWidth: 600 }}
        onFinish={onFinish}
        onFinishFailed={onFinishFailed}
        autoComplete="off"
        form={form}
      >
        <Form.Item<FieldType>
          label="Api-Key"
          name="api_key"
          rules={[{ required: true, message: 'Please input api key!' }]}
        >
          <Input />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default ApiKeyModal;
