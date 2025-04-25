import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Input, Modal } from 'antd';

interface IProps extends Omit<IModalManagerChildrenProps, 'showModal'> {
  loading: boolean;
  onOk: (name: string) => void;
  showModal?(): void;
}

const CreateAgentModal = ({ visible, hideModal, loading, onOk }: IProps) => {
  const [form] = Form.useForm();
  const { t } = useTranslate('common');

  type FieldType = {
    name?: string;
  };

  const handleOk = async () => {
    const ret = await form.validateFields();

    return onOk(ret.name);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      handleOk();
    }
  };

  return (
    <Modal
      title={t('createGraph', { keyPrefix: 'flow' })}
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
      okButtonProps={{ loading }}
      confirmLoading={loading}
    >
      <Form
        name="basic"
        labelCol={{ span: 4 }}
        wrapperCol={{ span: 20 }}
        style={{ maxWidth: 600 }}
        autoComplete="off"
        form={form}
      >
        <Form.Item<FieldType>
          label={t('name')}
          name="name"
          rules={[{ required: true, message: t('namePlaceholder') }]}
        >
          <Input onKeyDown={handleKeyDown} />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default CreateAgentModal;
