import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Modal } from 'antd';
import AsyncTreeSelect from './async-tree-select';

interface IProps extends Omit<IModalManagerChildrenProps, 'showModal'> {
  loading: boolean;
  onOk: (id: string) => void;
}

const FileMovingModal = ({ visible, hideModal, loading, onOk }: IProps) => {
  const [form] = Form.useForm();
  const { t } = useTranslate('fileManager');

  type FieldType = {
    name?: string;
  };

  const handleOk = async () => {
    const ret = await form.validateFields();

    return onOk(ret.name);
  };

  return (
    <Modal
      title={t('move', { keyPrefix: 'common' })}
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
      okButtonProps={{ loading }}
      confirmLoading={loading}
      width={600}
    >
      <Form
        name="basic"
        labelCol={{ span: 6 }}
        wrapperCol={{ span: 18 }}
        autoComplete="off"
        form={form}
      >
        <Form.Item<FieldType>
          label={t('destinationFolder')}
          name="name"
          rules={[{ required: true, message: t('pleaseSelect') }]}
        >
          <AsyncTreeSelect></AsyncTreeSelect>
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default FileMovingModal;
