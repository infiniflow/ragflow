import { Form, Input, Modal } from 'antd';

import { IModalProps } from '@/interfaces/common';

type FieldType = {
  username?: string;
};

const FeedbackModal = ({ visible, hideModal }: IModalProps<any>) => {
  const [form] = Form.useForm();

  const handleOk = async () => {
    const ret = await form.validateFields();
  };

  return (
    <Modal title="Feedback" open={visible} onOk={handleOk} onCancel={hideModal}>
      <Form
        name="basic"
        labelCol={{ span: 0 }}
        wrapperCol={{ span: 24 }}
        style={{ maxWidth: 600 }}
        autoComplete="off"
        form={form}
      >
        <Form.Item<FieldType>
          name="username"
          rules={[{ required: true, message: 'Please input your username!' }]}
        >
          <Input.TextArea rows={8} placeholder="Please input your username!" />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default FeedbackModal;
