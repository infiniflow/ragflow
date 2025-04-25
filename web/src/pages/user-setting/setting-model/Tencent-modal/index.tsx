import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { IAddLlmRequestBody } from '@/interfaces/request/llm';
import { Flex, Form, Input, Modal, Select, Space } from 'antd';
import omit from 'lodash/omit';

type FieldType = IAddLlmRequestBody & {
  TencentCloud_sid: string;
  TencentCloud_sk: string;
};

const { Option } = Select;

const TencentCloudModal = ({
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
    const modelType = values.model_type;

    const data = {
      ...omit(values),
      model_type: modelType,
      llm_factory: llmFactory,
      max_tokens: 16000,
    };
    console.info(data);

    onOk?.(data);
  };

  const handleKeyDown = async (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      await handleOk();
    }
  };

  return (
    <Modal
      title={t('addLlmTitle', { name: llmFactory })}
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
      okButtonProps={{ loading }}
      footer={(originNode: React.ReactNode) => {
        return (
          <Flex justify={'space-between'}>
            <a
              href={`https://cloud.tencent.com/document/api/1093/37823`}
              target="_blank"
              rel="noreferrer"
            >
              {t('TencentCloudLink')}
            </a>
            <Space>{originNode}</Space>
          </Flex>
        );
      }}
      confirmLoading={loading}
    >
      <Form>
        <Form.Item<FieldType>
          label={t('modelType')}
          name="model_type"
          initialValue={'speech2text'}
          rules={[{ required: true, message: t('modelTypeMessage') }]}
        >
          <Select placeholder={t('modelTypeMessage')}>
            <Option value="speech2text">speech2text</Option>
          </Select>
        </Form.Item>
        <Form.Item<FieldType>
          label={t('modelName')}
          name="llm_name"
          initialValue={'16k_zh'}
          rules={[{ required: true, message: t('SparkModelNameMessage') }]}
        >
          <Select placeholder={t('modelTypeMessage')}>
            <Option value="16k_zh">16k_zh</Option>
            <Option value="16k_zh_large">16k_zh_large</Option>
            <Option value="16k_multi_lang">16k_multi_lang</Option>
            <Option value="16k_zh_dialect">16k_zh_dialect</Option>
            <Option value="16k_en">16k_en</Option>
            <Option value="16k_yue">16k_yue</Option>
            <Option value="16k_zh-PY">16k_zh-PY</Option>
            <Option value="16k_ja">16k_ja</Option>
            <Option value="16k_ko">16k_ko</Option>
            <Option value="16k_vi">16k_vi</Option>
            <Option value="16k_ms">16k_ms</Option>
            <Option value="16k_id">16k_id</Option>
            <Option value="16k_fil">16k_fil</Option>
            <Option value="16k_th">16k_th</Option>
            <Option value="16k_pt">16k_pt</Option>
            <Option value="16k_tr">16k_tr</Option>
            <Option value="16k_ar">16k_ar</Option>
            <Option value="16k_es">16k_es</Option>
            <Option value="16k_hi">16k_hi</Option>
            <Option value="16k_fr">16k_fr</Option>
            <Option value="16k_zh_medical">16k_zh_medical</Option>
            <Option value="16k_de">16k_de</Option>
          </Select>
        </Form.Item>
        <Form.Item<FieldType>
          label={t('addTencentCloudSID')}
          name="TencentCloud_sid"
          rules={[{ required: true, message: t('TencentCloudSIDMessage') }]}
        >
          <Input
            placeholder={t('TencentCloudSIDMessage')}
            onKeyDown={handleKeyDown}
          />
        </Form.Item>
        <Form.Item<FieldType>
          label={t('addTencentCloudSK')}
          name="TencentCloud_sk"
          rules={[{ required: true, message: t('TencentCloudSKMessage') }]}
        >
          <Input
            placeholder={t('TencentCloudSKMessage')}
            onKeyDown={handleKeyDown}
          />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default TencentCloudModal;
