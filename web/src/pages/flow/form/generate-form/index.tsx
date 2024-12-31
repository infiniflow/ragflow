import LLMSelect from '@/components/llm-select';
import MessageHistoryWindowSizeItem from '@/components/message-history-window-size-item';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Input, Switch } from 'antd';
import { IOperatorForm } from '../../interface';
import DynamicParameters from './dynamic-parameters';

const GenerateForm = ({ onValuesChange, form, node }: IOperatorForm) => {
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
      <MessageHistoryWindowSizeItem
        initialValue={12}
      ></MessageHistoryWindowSizeItem>
      <DynamicParameters node={node}></DynamicParameters>
    </Form>
  );
};

export default GenerateForm;
