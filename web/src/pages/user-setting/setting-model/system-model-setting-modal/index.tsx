import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { LlmModelType } from '@/constants/knowledge';
import { useTranslate } from '@/hooks/commonHooks';
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
  const { t } = useTranslate('setting');

  const handleOk = async () => {
    const values = await form.validateFields();
    onOk(values);
  };

  useEffect(() => {
    if (visible) {
      form.setFieldsValue(initialValues);
    }
  }, [form, initialValues, visible]);

  const onFormLayoutChange = () => {};

  return (
    <Modal
      title={t('systemModelSettings')}
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
      okButtonProps={{ loading }}
      confirmLoading={loading}
    >
      <Form form={form} onValuesChange={onFormLayoutChange} layout={'vertical'}>
        <Form.Item
          label={t('chatModel')}
          name="llm_id"
          tooltip={t('chatModelTip')}
        >
          <Select options={allOptions[LlmModelType.Chat]} />
        </Form.Item>
        <Form.Item
          label={t('embeddingModel')}
          name="embd_id"
          tooltip={t('embeddingModelTip')}
        >
          <Select options={allOptions[LlmModelType.Embedding]} />
        </Form.Item>
        <Form.Item
          label={t('img2txtModel')}
          name="img2txt_id"
          tooltip={t('img2txtModelTip')}
        >
          <Select options={allOptions[LlmModelType.Image2text]} />
        </Form.Item>

        <Form.Item
          label={t('sequence2txtModel')}
          name="asr_id"
          tooltip={t('sequence2txtModelTip')}
        >
          <Select options={allOptions[LlmModelType.Speech2text]} />
        </Form.Item>
        <Form.Item
          label={t('rerankModel')}
          name="rerank_id"
          tooltip={t('rerankModelTip')}
        >
          <Select options={allOptions[LlmModelType.Rerank]} />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default SystemModelSettingModal;
