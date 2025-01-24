import LLMSelect from '@/components/llm-select';
import MessageHistoryWindowSizeItem from '@/components/message-history-window-size-item';
import { PromptEditor } from '@/components/prompt-editor';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Switch } from 'antd';
import { useMemo } from 'react';
import { useBuildComponentIdSelectOptions } from '../../hooks/use-get-begin-query';
import { IOperatorForm } from '../../interface';
import DynamicParameters from './dynamic-parameters';

const list = [
  {
    value: 'afc163',
    label: 'afc163',
  },
  {
    value: 'zombieJ',
    label: 'zombieJ',
  },
  {
    value: 'yesmeck',
    label: 'yesmeck',
  },
].map((x) => ({
  ...x,
  value: `{${x.value}}`,
}));

const GenerateForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslate('flow');

  const options = useBuildComponentIdSelectOptions(node?.id, node?.parentId);

  const nextOptions = useMemo(() => {
    return options.reduce<any[]>((pre, cur) => {
      cur.options.forEach((x) => {
        pre.push({ ...x, value: `{${x.value}}` });
      });
      return pre;
    }, []);
  }, [options]);

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
      <DynamicParameters node={node}></DynamicParameters>
    </Form>
  );
};

export default GenerateForm;
