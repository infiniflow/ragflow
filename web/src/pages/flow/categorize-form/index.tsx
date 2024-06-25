import LLMSelect from '@/components/llm-select';
import { useTranslate } from '@/hooks/commonHooks';
import { Form } from 'antd';
import { IOperatorForm } from '../interface';
import DynamicCategorize from './dynamic-categorize';

const CategorizeForm = ({ form, onValuesChange }: IOperatorForm) => {
  const { t } = useTranslate('flow');

  return (
    <Form
      name="basic"
      labelCol={{ span: 9 }}
      wrapperCol={{ span: 15 }}
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
      initialValues={{ items: [{}] }}
      // layout={'vertical'}
    >
      <Form.Item name={['cite']} label={t('cite')} tooltip={t('citeTip')}>
        <LLMSelect></LLMSelect>
      </Form.Item>
      <DynamicCategorize></DynamicCategorize>
    </Form>
  );
};

export default CategorizeForm;
