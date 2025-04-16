import React from 'react';
import { Modal, Form, Input, Upload } from 'antd';
import { useTranslation } from 'react-i18next';
import { UploadOutlined } from '@ant-design/icons';

interface EditModalProps {
  visible: boolean;
  title: string;
  initialValues: {
    id: string;
    name: string;
  };
  onOk: (values: any) => void;
  onCancel: () => void;
}

const EditModal: React.FC<EditModalProps> = ({
  visible,
  title,
  initialValues,
  onOk,
  onCancel,
}) => {
  const { t } = useTranslation();
  const [form] = Form.useForm();

  React.useEffect(() => {
    if (visible && initialValues) {
      form.setFieldsValue(initialValues);
    }
  }, [visible, initialValues, form]);

  const handleOk = () => {
    form.validateFields().then((values) => {
      onOk({
        ...values,
        id: initialValues.id,
      });
      form.resetFields();
    });
  };

  const handleCancel = () => {
    form.resetFields();
    onCancel();
  };

  const uploadProps = {
    name: 'file',
    action: 'https://run.mocky.io/v3/435e224c-44fb-4773-9faf-380c5e6a2188',
    headers: {
      authorization: 'authorization-text',
    },
    accept: 'image/*',
  };

  return (
    <Modal
      title={title}
      open={visible}
      onOk={handleOk}
      onCancel={handleCancel}
      okText="保存"
      cancelText="取消"
    >
      <Form form={form} layout="vertical">
        <Form.Item
          name="name"
          label="名称"
          rules={[{ required: true, message: '请输入名称' }]}
        >
          <Input placeholder="请输入名称" />
        </Form.Item>
        <Form.Item
          name="avatar"
          label="头像"
        >
          <Upload
            {...uploadProps}
            maxCount={1}
            listType="picture-card"
          >
            <div>
              <UploadOutlined />
              <div style={{ marginTop: 8 }}>
                Drag 'n' drop files here, or click to select files
              </div>
              <div style={{ fontSize: 12, color: '#999' }}>
                You can upload a file with 4 MB
              </div>
            </div>
          </Upload>
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default EditModal;
