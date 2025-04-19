import LLMSelect from '@/components/llm-select';
import { useTranslate } from '@/hooks/common-hooks';
import DynamicVariablesForm from '@/pages/flow/form/mcp-sse-client-form/dynamic-variables';
import { Form } from 'antd';
import { IOperatorForm } from '../../interface';
import DynamicInputVariable from '../components/dynamic-input-variable';

const McpSseClientForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslate('mcp');

  return (
    <Form
      name="basic"
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
      layout={'vertical'}
    >
      <DynamicInputVariable node={node}></DynamicInputVariable>
      <Form.Item
        name={'llm_id'}
        label={t('chatModel')}
        tooltip={t('chatModelTip')}
      >
        <LLMSelect></LLMSelect>
      </Form.Item>
      <DynamicVariablesForm node={node}></DynamicVariablesForm>
    </Form>
  );
};

export default McpSseClientForm;
