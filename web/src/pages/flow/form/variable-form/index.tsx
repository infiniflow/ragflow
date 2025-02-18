import LLMSelect from '@/components/llm-select';
import MessageHistoryWindowSizeItem from '@/components/message-history-window-size-item';
import { Editor } from '@monaco-editor/react';
import { Form } from 'antd';
import { useTranslation } from 'react-i18next';
import { IOperatorForm } from '../../interface';

const VariableForm = ({ onValuesChange, form }: IOperatorForm) => {
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
        <Form.Item name={'variables'} label={t('flow.variables')}>
          <Editor height={200} defaultLanguage="json" theme="vs-dark" />
        </Form.Item>
        <MessageHistoryWindowSizeItem
          initialValue={6}
        ></MessageHistoryWindowSizeItem>
      </Form>
    </>
  );
};

export default VariableForm;
