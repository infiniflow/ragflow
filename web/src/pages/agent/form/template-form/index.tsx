import { PromptEditor } from '@/components/prompt-editor';
import { Form } from 'antd';
import { useTranslation } from 'react-i18next';
import { IOperatorForm } from '../../interface';

const TemplateForm = ({ onValuesChange, form }: IOperatorForm) => {
  const { t } = useTranslation();

  return (
    <Form
      name="basic"
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
      layout={'vertical'}
    >
      <Form.Item name={['content']} label={t('flow.content')}>
        <PromptEditor></PromptEditor>
      </Form.Item>
    </Form>
  );
};

export default TemplateForm;
