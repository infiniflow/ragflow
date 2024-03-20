import { Form, Input, Modal } from 'antd';
import { useEffect } from 'react';
import { IModalManagerChildrenProps } from '../modal-manager';

interface IProps extends Omit<IModalManagerChildrenProps, 'showModal'> {
  loading: boolean;
  initialName: string;
  onOk: (name: string) => void;
  showModal?(): void;
}

const RenameModal = ({
  visible,
  hideModal,
  loading,
  initialName,
  onOk,
}: IProps) => {
  const [form] = Form.useForm();

  type FieldType = {
    name?: string;
  };

  const handleOk = async () => {
    const ret = await form.validateFields();

    return onOk(ret.name);
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
      form.setFieldValue('name', initialName);
    }
  }, [initialName, form, visible]);

  return (
    <Modal
      title="Rename"
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
          label="Name"
          name="name"
          rules={[{ required: true, message: 'Please input name!' }]}
        >
          <Input />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default RenameModal;
