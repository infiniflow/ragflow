import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { Form, Input, Modal, Select } from 'antd';
import React from 'react';

type FieldType = {
  name?: string;
  fileType?: string;
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
    const fileName = `${values.name}.${values.fileType}`;
    onOk(fileName);
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
        <Form.Item<FieldType> label="File Name" style={{ marginBottom: 0 }}>
          <Input.Group compact>
            <Form.Item
              name="name"
              noStyle
              rules={[{ required: true, message: 'Please input name!' }]}
            >
              <Input style={{ width: '80%' }} />
            </Form.Item>
            <Form.Item name="fileType" initialValue="txt" noStyle>
              <Select style={{ width: '20%' }}>
                <Select.Option value="txt">.txt</Select.Option>
                <Select.Option value="md">.md</Select.Option>
                <Select.Option value="json">.json</Select.Option>
              </Select>
            </Form.Item>
          </Input.Group>
        </Form.Item>
      </Form>
    </Modal>
  );
};
export default FileCreatingModal;
