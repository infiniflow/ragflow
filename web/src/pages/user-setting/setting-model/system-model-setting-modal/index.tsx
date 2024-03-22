import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { LlmModelType } from '@/constants/knowledge';
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
  const { systemSetting: initialValues, allOptions } =
    useFetchSystemModelSettingOnMount(visible);

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
      title="System Model Settings"
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
      okButtonProps={{ loading }}
      confirmLoading={loading}
    >
      <Form form={form} onValuesChange={onFormLayoutChange} layout={'vertical'}>
        
      <Form.Item label="Chat model" name="llm_id" tooltip="The default chat LLM all the newly created knowledgebase will use.">
          <Select options={allOptions[LlmModelType.Chat]} />
        </Form.Item>
        <Form.Item label="Embedding model" name="embd_id" tooltip="The default embedding model all the newly created knowledgebase will use.">
          <Select options={allOptions[LlmModelType.Embedding]} />
        </Form.Item>
        <Form.Item
          label="Img2txt model"
          name="img2txt_id"
          tooltip="The default multi-module model all the newly created knowledgebase will use. It can describe a picture or video."
        >
          <Select options={allOptions[LlmModelType.Image2text]} />
        </Form.Item>
        
        <Form.Item
          label="Sequence2txt model"
          name="asr_id"
          tooltip="The default ASR model all the newly created knowledgebase will use. Use this model to translate voices to corresponding text."
        >
          <Select options={allOptions[LlmModelType.Speech2text]} />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default SystemModelSettingModal;
