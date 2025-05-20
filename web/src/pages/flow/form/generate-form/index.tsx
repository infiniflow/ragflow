import LLMSelect from '@/components/llm-select';
import MessageHistoryWindowSizeItem from '@/components/message-history-window-size-item';
import { PromptEditor } from '@/components/prompt-editor';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Switch } from 'antd';
import { IOperatorForm } from '../../interface';

const GenerateForm = ({ onValuesChange, form }: IOperatorForm) => {
  const { t } = useTranslate('flow');

  return (
    <Form
      name="basic"
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
      layout={'vertical'}
    >
      <Form.Item
        name={'llm_id'}
        label={t('model', { keyPrefix: 'chat' })}
        tooltip={t('modelTip', { keyPrefix: 'chat' })}
      >
        <LLMSelect></LLMSelect>
      </Form.Item>
      <Form.Item
        name={['prompt']}
        label={t('systemPrompt')}
        initialValue={t('promptText')}
        tooltip={t('promptTip')}
        rules={[
          {
            required: true,
            message: t('promptMessage'),
          },
        ]}
      >
        {/* <Input.TextArea rows={8}></Input.TextArea> */}
        <PromptEditor></PromptEditor>
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
      <MessageHistoryWindowSizeItem
        initialValue={12}
      ></MessageHistoryWindowSizeItem>
    </Form>
  );
};

export default GenerateForm;
