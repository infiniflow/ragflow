import LLMSelect from '@/components/llm-select';
import MessageHistoryWindowSizeItem from '@/components/message-history-window-size-item';
import { loader } from '@monaco-editor/react';
import { Form } from 'antd';
import { useTranslation } from 'react-i18next';
import { IOperatorForm } from '../../interface';
import DynamicVariablesForm from './dynamic-variables';
loader.config({ paths: { vs: '/vs' } });

const VariablesForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslation();

  return (
    <>
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
        <MessageHistoryWindowSizeItem
          initialValue={6}
        ></MessageHistoryWindowSizeItem>
        <DynamicVariablesForm node={node}></DynamicVariablesForm>
      </Form>
    </>
  );
};

export default VariablesForm;
