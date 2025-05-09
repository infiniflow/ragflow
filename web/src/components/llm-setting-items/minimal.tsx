import { LlmModelType } from '@/constants/knowledge';
import { useTranslate } from '@/hooks/common-hooks';
import { useComposeLlmOptionsByModelTypes } from '@/hooks/llm-hooks';
import { Form, Select } from 'antd';

export default function MinimalLlmSettingItems({ namePrefix = '' }) {
  const { t } = useTranslate('chat');
  const modelOptions = useComposeLlmOptionsByModelTypes([
    LlmModelType.Chat,
    LlmModelType.Image2text,
  ]);

  return (
    <Form.Item
      label={t('model')}
      name={`${namePrefix}llm_id`} /* keeps the same field path */
      rules={[{ required: true, message: t('modelMessage') }]}
      tooltip={t('modelTip')}
    >
      <Select options={modelOptions} showSearch popupMatchSelectWidth={false} />
    </Form.Item>
  );
}
