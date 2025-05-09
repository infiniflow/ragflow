import Editor, { loader } from '@monaco-editor/react';
import { Form } from 'antd';
import { useTranslation } from 'react-i18next';
import { IOperatorForm } from '../../interface';
import DynamicInputVariable from '../components/dynamic-input-variable';
import { DynamicOutputVariable } from './dynamic-output-variable';

loader.config({ paths: { vs: '/vs' } });

const CodeForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslation();

  return (
    <Form
      name="basic"
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
      layout={'vertical'}
    >
      <DynamicInputVariable node={node}></DynamicInputVariable>
      <Form.Item name={'code'} label={t('flow.code')}>
        <Editor height={200} defaultLanguage="python" theme="vs-dark" />
      </Form.Item>
      <DynamicOutputVariable></DynamicOutputVariable>
    </Form>
  );
};

export default CodeForm;
