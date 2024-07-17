import LLMSelect from '@/components/llm-select';
import { useTranslate } from '@/hooks/common-hooks';
import { Form } from 'antd';
import { useSetLlmSetting } from '../hooks';
import { IOperatorForm } from '../interface';
import DynamicCategorize from './dynamic-categorize';
import { useHandleFormValuesChange } from './hooks';

const CategorizeForm = ({ form, onValuesChange, node }: IOperatorForm) => {
  const { t } = useTranslate('flow');
  const { handleValuesChange } = useHandleFormValuesChange({
    form,
    nodeId: node?.id,
    onValuesChange,
  });
  useSetLlmSetting(form);

  return (
    <Form
      name="basic"
      labelCol={{ span: 6 }}
      wrapperCol={{ span: 18 }}
      autoComplete="off"
      form={form}
      onValuesChange={handleValuesChange}
      initialValues={{ items: [{}] }}
    >
      <Form.Item
        name={'llm_id'}
        label={t('model', { keyPrefix: 'chat' })}
        tooltip={t('modelTip', { keyPrefix: 'chat' })}
      >
        <LLMSelect></LLMSelect>
      </Form.Item>
      <DynamicCategorize nodeId={node?.id}></DynamicCategorize>
    </Form>
  );
};

export default CategorizeForm;
