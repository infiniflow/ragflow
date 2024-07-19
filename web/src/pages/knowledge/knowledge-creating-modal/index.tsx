import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { Form, Input, Modal } from 'antd';
import { useTranslation } from 'react-i18next';

type FieldType = {
  name?: string;
};

interface IProps extends Omit<IModalManagerChildrenProps, 'showModal'> {
  loading: boolean;
  onOk: (name: string) => void;
}

const KnowledgeCreatingModal = ({
  visible,
  hideModal,
  loading,
  onOk,
}: IProps) => {
  const [form] = Form.useForm();

  const { t } = useTranslation('translation', { keyPrefix: 'knowledgeList' });

  const handleOk = async () => {
    const ret = await form.validateFields();

    onOk(ret.name);
  };

  return (
    <Modal
      title={t('createKnowledgeBase')}
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
      okButtonProps={{ loading }}
    >
      <Form
        name="Create"
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
          <Input placeholder={t('namePlaceholder')} />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default KnowledgeCreatingModal;
