import Editor, { loader } from '@monaco-editor/react';
import { Form, Select } from 'antd';
import { IOperatorForm } from '../../interface';
import { DynamicInputVariable } from './dynamic-input-variable';

import { CodeTemplateStrMap, ProgrammingLanguage } from '@/constants/agent';
import { ICodeForm } from '@/interfaces/database/flow';
import { useEffect } from 'react';
import styles from './index.less';

loader.config({ paths: { vs: '/vs' } });

const options = [
  ProgrammingLanguage.Python,
  ProgrammingLanguage.Javascript,
].map((x) => ({ value: x, label: x }));

const CodeForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const formData = node?.data.form as ICodeForm;

  useEffect(() => {
    setTimeout(() => {
      // TODO: Direct operation zustand is more elegant
      form?.setFieldValue(
        'script',
        CodeTemplateStrMap[formData.lang as ProgrammingLanguage],
      );
    }, 0);
  }, [form, formData.lang]);

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
        name={'script'}
        label={
          <Form.Item name={'lang'} className={styles.languageItem}>
            <Select
              defaultValue={'python'}
              popupMatchSelectWidth={false}
              options={options}
            />
          </Form.Item>
        }
        className="bg-gray-100 rounded dark:bg-gray-800"
      >
        <Editor
          height={200}
          theme="vs-dark"
          language={formData.lang}
          options={{
            minimap: { enabled: false },
            automaticLayout: true,
          }}
        />
      </Form.Item>
    </Form>
  );
};

export default CodeForm;
