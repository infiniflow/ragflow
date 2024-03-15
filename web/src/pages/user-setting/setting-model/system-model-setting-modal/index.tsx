import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { ISystemModelSettingSavingParams } from '@/hooks/llmHooks';
import { Form, Modal, Select } from 'antd';
import { useEffect } from 'react';
import { useFetchSystemModelSettingOnMount } from '../hooks';

interface IProps extends Omit<IModalManagerChildrenProps, 'showModal'> {
  loading: boolean;
  onOk: (
    payload: Omit<ISystemModelSettingSavingParams, 'tenant_id' | 'name'>,
  ) => void;
}

const SystemModelSettingModal = ({
  visible,
  hideModal,
  onOk,
  loading,
}: IProps) => {
  const [form] = Form.useForm();
  const initialValues = useFetchSystemModelSettingOnMount();

  const handleOk = async () => {
    const values = await form.validateFields();
    onOk(values);
  };

  useEffect(() => {
    form.setFieldsValue(initialValues);
  }, [form, initialValues]);

  const onFormLayoutChange = () => {};

  return (
    <Modal
      title="Basic Modal"
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
      okButtonProps={{ loading }}
      confirmLoading={loading}
    >
      <Form form={form} onValuesChange={onFormLayoutChange} layout={'vertical'}>
        <Form.Item label="sequence2txt model" name="asr_id">
          <Select options={[{ value: 'sample', label: <span>sample</span> }]} />
        </Form.Item>
        <Form.Item label="Embedding model" name="embd_id">
          <Select options={[{ value: 'sample', label: <span>sample</span> }]} />
        </Form.Item>
        <Form.Item label="img2txt_id model" name="img2txt_id">
          <Select options={[{ value: 'sample', label: <span>sample</span> }]} />
        </Form.Item>
        <Form.Item label="Chat model" name="llm_id">
          <Select options={[{ value: 'sample', label: <span>sample</span> }]} />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default SystemModelSettingModal;
