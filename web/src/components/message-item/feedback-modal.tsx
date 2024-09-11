import { Form, Input, Modal } from 'antd';

import { IModalProps } from '@/interfaces/common';
import { IFeedbackRequestBody } from '@/interfaces/request/chat';
import { useCallback } from 'react';

type FieldType = {
  feedback?: string;
};

const FeedbackModal = ({
  visible,
  hideModal,
  onOk,
  loading,
}: IModalProps<IFeedbackRequestBody>) => {
  const [form] = Form.useForm();

  const handleOk = useCallback(async () => {
    const ret = await form.validateFields();
    return onOk?.({ thumbup: false, feedback: ret.feedback });
  }, [onOk, form]);

  return (
    <Modal
      title="Feedback"
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
      confirmLoading={loading}
    >
      <Form
        name="basic"
        labelCol={{ span: 0 }}
        wrapperCol={{ span: 24 }}
        style={{ maxWidth: 600 }}
        autoComplete="off"
        form={form}
      >
        <Form.Item<FieldType>
          name="feedback"
          rules={[{ required: true, message: 'Please input your feedback!' }]}
        >
          <Input.TextArea rows={8} placeholder="Please input your feedback!" />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default FeedbackModal;
