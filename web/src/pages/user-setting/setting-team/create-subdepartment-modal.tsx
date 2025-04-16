import React from 'react';
import { Modal, Form, Input } from 'antd';
import { useTranslation } from 'react-i18next';

interface CreateSubDepartmentModalProps {
  visible: boolean;
  parentDepartmentId: string;
  parentDepartmentName: string;
  onOk: (values: any) => void;
  onCancel: () => void;
}

const CreateSubDepartmentModal: React.FC<CreateSubDepartmentModalProps> = ({
  visible,
  parentDepartmentId,
  parentDepartmentName,
  onOk,
  onCancel,
}) => {
  const { t } = useTranslation();
  const [form] = Form.useForm();

  const handleOk = () => {
    form.validateFields().then((values) => {
      onOk({
        ...values,
        parentDepartmentId,
      });
      form.resetFields();
    });
  };

  const handleCancel = () => {
    form.resetFields();
    onCancel();
  };

  return (
    <Modal
      title={t('创建"{{name}}"的子部门', { name: parentDepartmentName })}
      open={visible}
      onOk={handleOk}
      onCancel={handleCancel}
      okText={t('确认')}
      cancelText={t('取消')}
    >
      <Form form={form} layout="vertical">
        <Form.Item
          name="name"
          label={t('子部门名称')}
          rules={[{ required: true, message: t('请输入子部门名称') }]}
        >
          <Input placeholder={t('请输入子部门名称')} />
        </Form.Item>
        <Form.Item
          name="description"
          label={t('子部门描述')}
        >
          <Input.TextArea placeholder={t('请输入子部门描述')} rows={4} />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default CreateSubDepartmentModal;
