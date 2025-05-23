import Editor, { loader } from '@monaco-editor/react';
import { Form, Select } from 'antd';
import { IOperatorForm } from '../../interface';
import { DynamicInputVariable } from './dynamic-input-variable';

import { CodeTemplateStrMap, ProgrammingLanguage } from '@/constants/agent';
import { ICodeForm } from '@/interfaces/database/flow';
import { useCallback } from 'react';
import useGraphStore from '../../store';
import styles from './index.less';

loader.config({ paths: { vs: '/vs' } });

const options = [
  ProgrammingLanguage.Python,
  ProgrammingLanguage.Javascript,
].map((x) => ({ value: x, label: x }));

const CodeForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const formData = node?.data.form as ICodeForm;
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);

  const handleChange = useCallback(
    (value: ProgrammingLanguage) => {
      if (node?.id) {
        updateNodeForm(
          node?.id,
          CodeTemplateStrMap[value as ProgrammingLanguage],
          ['script'],
        );
      }
    },
    [node?.id, updateNodeForm],
  );

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
              defaultValue={ProgrammingLanguage.Python}
              popupMatchSelectWidth={false}
              options={options}
              onChange={handleChange}
            />
          </Form.Item>
        }
        className="bg-gray-100 rounded dark:bg-gray-800"
      >
        <Editor
          height={600}
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
