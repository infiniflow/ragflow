import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Input, Modal } from 'antd';
import React from 'react';

interface IProps extends Omit<IModalManagerChildrenProps, 'showModal'> {
  loading: boolean;
  onOk: (name: string, url: string) => void;
  showModal?(): void;
}

const WebCrawlModal: React.FC<IProps> = ({ visible, hideModal, onOk }) => {
  const [form] = Form.useForm();
  const { t } = useTranslate('knowledgeDetails');
  const handleOk = async () => {
    const values = await form.validateFields();
    onOk(values.name, values.url);
  };

  return (
    <Modal
      title={t('webCrawl')}
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
        <Form.Item
          label="Name"
          name="name"
          rules={[
            { required: true, message: 'Please input name!' },
            {
              max: 10,
              message: 'The maximum length of name is 128 characters',
            },
          ]}
        >
          <Input placeholder="Document name" />
        </Form.Item>
        <Form.Item
          label="URL"
          name="url"
          rules={[
            { required: true, message: 'Please input url!' },
            {
              pattern: new RegExp(
                '(https?|ftp|file)://[-A-Za-z0-9+&@#/%?=~_|!:,.;]+[-A-Za-z0-9+&@#/%=~_|]',
              ),
              message: 'Please enter a valid URL!',
            },
          ]}
        >
          <Input placeholder="https://www.baidu.com" />
        </Form.Item>
      </Form>
    </Modal>
  );
};
export default WebCrawlModal;
