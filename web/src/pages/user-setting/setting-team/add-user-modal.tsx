import { IModalProps } from '@/interfaces/common';
import { Form, Input, Modal } from 'antd';
import { useTranslation } from 'react-i18next';

const AddingUserModal = ({
  visible,
  hideModal,
  loading,
  onOk,
}: IModalProps<string>) => {
  const [form] = Form.useForm();
  const { t } = useTranslation();

  type FieldType = {
    email?: string;
  };

  const handleOk = async () => {
    const ret = await form.validateFields();

    return onOk?.(ret.email);
  };

  return (
    <Modal
      title={t('setting.add')}
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
      okButtonProps={{ loading }}
      confirmLoading={loading}
    >
      <Form
        name="basic"
        labelCol={{ span: 6 }}
        wrapperCol={{ span: 18 }}
        autoComplete="off"
        form={form}
      >
        <Form.Item<FieldType>
          label={t('setting.email')}
          name="email"
          rules={[{ required: true }]}
        >
          <Input />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default AddingUserModal;
