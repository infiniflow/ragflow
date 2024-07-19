import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Input, Modal } from 'antd';
import { useEffect } from 'react';

interface IProps extends Omit<IModalManagerChildrenProps, 'showModal'> {
  loading: boolean;
  initialName: string;
  onOk: (name: string) => void;
  showModal?(): void;
}

const RenameModal = ({
  visible,
  onOk,
  loading,
  initialName,
  hideModal,
}: IProps) => {
  const [form] = Form.useForm();
  const { t } = useTranslate('common');
  type FieldType = {
    name?: string;
  };

  const handleOk = async () => {
    const ret = await form.validateFields();
    onOk(ret.name);
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
      title={t('rename')}
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
      okButtonProps={{ loading }}
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
          label={t('name')}
          name="name"
          rules={[{ required: true, message: t('namePlaceholder') }]}
        >
          <Input />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default RenameModal;
