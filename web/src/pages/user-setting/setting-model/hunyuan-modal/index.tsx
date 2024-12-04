import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { IAddLlmRequestBody } from '@/interfaces/request/llm';
import { Form, Input, Modal, Select } from 'antd';
import omit from 'lodash/omit';

type FieldType = IAddLlmRequestBody & {
  vision: boolean;
  hunyuan_sid: string;
  hunyuan_sk: string;
};

const { Option } = Select;

const HunyuanModal = ({
  visible,
  hideModal,
  onOk,
  loading,
  llmFactory,
}: IModalProps<IAddLlmRequestBody> & { llmFactory: string }) => {
  const [form] = Form.useForm<FieldType>();

  const { t } = useTranslate('setting');

  const handleOk = async () => {
    const values = await form.validateFields();
    const modelType =
      values.model_type === 'chat' && values.vision
        ? 'image2text'
        : values.model_type;

    const data = {
      ...omit(values, ['vision']),
      model_type: modelType,
      llm_factory: llmFactory,
    };
    console.info(data);

    onOk?.(data);
  };

  return (
    <Modal
      title={t('addLlmTitle', { name: llmFactory })}
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
      okButtonProps={{ loading }}
      confirmLoading={loading}
    >
      <Form
        name="basic"
        style={{ maxWidth: 600 }}
        autoComplete="off"
        layout={'vertical'}
        form={form}
      >
        <Form.Item<FieldType>
          label={t('addHunyuanSID')}
          name="hunyuan_sid"
          rules={[{ required: true, message: t('HunyuanSIDMessage') }]}
        >
          <Input placeholder={t('HunyuanSIDMessage')} />
        </Form.Item>
        <Form.Item<FieldType>
          label={t('addHunyuanSK')}
          name="hunyuan_sk"
          rules={[{ required: true, message: t('HunyuanSKMessage') }]}
        >
          <Input placeholder={t('HunyuanSKMessage')} />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default HunyuanModal;
