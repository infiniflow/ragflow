import LlmSettingItems from '@/components/llm-setting-items';
import { useTranslate } from '@/hooks/commonHooks';
import { Form, Input, Switch } from 'antd';
import { useSetLlmSetting } from '../hooks';
import { IOperatorForm } from '../interface';

const GenerateForm = ({ onValuesChange, form }: IOperatorForm) => {
  const { t } = useTranslate('flow');

  useSetLlmSetting(form);

  return (
    <Form
      name="basic"
      labelCol={{ span: 9 }}
      wrapperCol={{ span: 15 }}
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
    >
      <LlmSettingItems></LlmSettingItems>
      <Form.Item
        name={['prompt']}
        label={t('prompt', { keyPrefix: 'knowledgeConfiguration' })}
        initialValue={t('promptText', { keyPrefix: 'knowledgeConfiguration' })}
        tooltip={t('promptTip', { keyPrefix: 'knowledgeConfiguration' })}
        rules={[
          {
            required: true,
            message: t('promptMessage'),
          },
        ]}
      >
        <Input.TextArea rows={8} />
      </Form.Item>
      <Form.Item
        name={['cite']}
        label={t('cite')}
        initialValue={true}
        valuePropName="checked"
        tooltip={t('citeTip')}
      >
        <Switch />
      </Form.Item>
    </Form>
  );
};

export default GenerateForm;
