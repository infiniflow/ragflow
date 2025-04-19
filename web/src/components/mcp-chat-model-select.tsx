import { LlmModelType } from '@/constants/knowledge';
import { useTranslate } from '@/hooks/common-hooks';
import { useFetchSystemModelSettingOnMount } from '@/pages/user-setting/setting-model/hooks';
import { Form, Select } from 'antd';
import { useEffect } from 'react';

const MCPChatModelSelect = () => {
  const [form] = Form.useForm();
  const { systemSetting: initialValues, allOptions } =
    useFetchSystemModelSettingOnMount();
  const { t } = useTranslate('mcp');

  useEffect(() => {
    form.setFieldsValue(initialValues);
  }, [form, initialValues]);

  return (
    <Form form={form} layout={'vertical'}>
      <Form.Item
        label={t('chatModel')}
        name="llm_id"
        tooltip={t('chatModelTip')}
      >
        <Select
          options={[...allOptions[LlmModelType.Chat]]}
          allowClear
          showSearch
        />
      </Form.Item>
    </Form>
  );
};

export default MCPChatModelSelect;
