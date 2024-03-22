import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { Form, Input, Modal } from 'antd';
import React from 'react';

type FieldType = {
  name?: string;
};

interface IProps extends Omit<IModalManagerChildrenProps, 'showModal'> {
  loading: boolean;
  onOk: (name: string) => void;
  showModal?(): void;
}

const FileCreatingModal: React.FC<IProps> = ({ visible, hideModal, onOk }) => {
  const [form] = Form.useForm();

  const handleOk = async () => {
    const values = await form.validateFields();
    onOk(values.name);
  };

  return (
    <Modal
      title="File Name"
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
    >
      <Form
        form={form}
        name="validateOnly"
        labelCol={{ span: 4 }}
        wrapperCol={{ span: 20 }}
        style={{ maxWidth: 600 }}
        autoComplete="off"
      >
        <Form.Item<FieldType>
          label="File Name"
          name="name"
          rules={[{ required: true, message: 'Please input name!' }]}
        >
          <Input />
        </Form.Item>
      </Form>
    </Modal>
  );
};
export default FileCreatingModal;
