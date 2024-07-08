import LLMSelect from '@/components/llm-select';
import { useTranslate } from '@/hooks/commonHooks';
import { Form, Input, Switch } from 'antd';
import { useSetLlmSetting } from '../hooks';
import { IOperatorForm } from '../interface';
import DynamicParameters from './dynamic-parameters';

const GenerateForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslate('flow');

  useSetLlmSetting(form);

  return (
    <Form
      name="basic"
      labelCol={{ span: 5 }}
      wrapperCol={{ span: 19 }}
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
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
        label={t('prompt', { keyPrefix: 'knowledgeConfiguration' })}
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
      <DynamicParameters nodeId={node?.id}></DynamicParameters>
    </Form>
  );
};

export default GenerateForm;
