import LLMSelect from '@/components/llm-select';
import MessageHistoryWindowSizeItem from '@/components/message-history-window-size-item';
import { useTranslate } from '@/hooks/common-hooks';
import { Form } from 'antd';
import { IOperatorForm } from '../../interface';
import DynamicInputVariable from '../components/dynamic-input-variable';
import DynamicCategorize from './dynamic-categorize';
import { useHandleFormValuesChange } from './hooks';

const CategorizeForm = ({ form, onValuesChange, node }: IOperatorForm) => {
  const { t } = useTranslate('flow');
  const { handleValuesChange } = useHandleFormValuesChange({
    form,
    nodeId: node?.id,
    onValuesChange,
  });

  return (
    <Form
      name="basic"
      autoComplete="off"
      form={form}
      onValuesChange={handleValuesChange}
      initialValues={{ items: [{}] }}
      layout={'vertical'}
    >
      <DynamicInputVariable node={node}></DynamicInputVariable>
      <Form.Item
        name={'llm_id'}
        label={t('model', { keyPrefix: 'chat' })}
        tooltip={t('modelTip', { keyPrefix: 'chat' })}
      >
        <LLMSelect></LLMSelect>
      </Form.Item>
      <MessageHistoryWindowSizeItem
        initialValue={1}
      ></MessageHistoryWindowSizeItem>
      <DynamicCategorize nodeId={node?.id}></DynamicCategorize>
    </Form>
  );
};

export default CategorizeForm;
