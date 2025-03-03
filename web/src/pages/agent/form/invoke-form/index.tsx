import Editor, { loader } from '@monaco-editor/react';
import { Form, Input, InputNumber, Select, Space, Switch } from 'antd';
import { useTranslation } from 'react-i18next';
import { IOperatorForm } from '../../interface';
import DynamicVariablesForm from './dynamic-variables';

loader.config({ paths: { vs: '/vs' } });

enum Method {
  GET = 'GET',
  POST = 'POST',
  PUT = 'PUT',
}

const MethodOptions = [Method.GET, Method.POST, Method.PUT].map((x) => ({
  label: x,
  value: x,
}));

interface TimeoutInputProps {
  value?: number;
  onChange?: (value: number | null) => void;
}

const TimeoutInput = ({ value, onChange }: TimeoutInputProps) => {
  const { t } = useTranslation();
  return (
    <Space>
      <InputNumber value={value} onChange={onChange} /> {t('common.s')}
    </Space>
  );
};

const InvokeForm = ({ onValuesChange, form, node }: IOperatorForm) => {
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
        <Form.Item name={'url'} label={t('flow.url')}>
          <Input />
        </Form.Item>
        <Form.Item
          name={'method'}
          label={t('flow.method')}
          initialValue={Method.GET}
        >
          <Select options={MethodOptions} />
        </Form.Item>
        <Form.Item name={'timeout'} label={t('flow.timeout')}>
          <TimeoutInput></TimeoutInput>
        </Form.Item>
        <Form.Item name={'headers'} label={t('flow.headers')}>
          <Editor height={200} defaultLanguage="json" theme="vs-dark" />
        </Form.Item>
        <Form.Item name={'proxy'} label={t('flow.proxy')}>
          <Input />
        </Form.Item>
        <Form.Item
          name={'clean_html'}
          label={t('flow.cleanHtml')}
          tooltip={t('flow.cleanHtmlTip')}
        >
          <Switch />
        </Form.Item>
        <DynamicVariablesForm node={node}></DynamicVariablesForm>
      </Form>
    </>
  );
};

export default InvokeForm;
