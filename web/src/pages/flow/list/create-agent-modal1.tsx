import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Input, Modal, Select } from 'antd';
import { useSelectKnowledgeOptions } from '@/hooks/knowledge-hooks';

interface IProps extends Omit<IModalManagerChildrenProps, 'showModal'> {
  loading: boolean;
  onOk: (values: { name: string; kb_ids: string[] }) => void;
  showModal?(): void;
}

type FieldType = {
  name: string;
  kb_ids: string[];
};

const CreateAgentModal = ({ visible, hideModal, loading, onOk }: IProps) => {
  const [form] = Form.useForm<FieldType>();
  const { t } = useTranslate('common');
  const options = useSelectKnowledgeOptions();

  const handleOk = async () => {
    const values = await form.validateFields();
    return onOk(values);
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
          <Input />
        </Form.Item>

        <Form.Item<FieldType>
          label={t('knowledgeBases')}
          name="kb_ids"
          rules={[{ required: true, message: t('selectKnowledgeBase') }]}
        >
          <Select
            mode="multiple"
            
            options={options}
          />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default CreateAgentModal;
