import { Form, Input } from 'antd';
import { useTranslation } from 'react-i18next';
import { IOperatorForm } from '../../interface';
import DynamicParameters from '../generate-form/dynamic-parameters';

const TemplateForm = ({ onValuesChange, form, node }: IOperatorForm) => {
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
        <Input.TextArea rows={8} placeholder={t('flow.blank')} />
      </Form.Item>

      <DynamicParameters node={node}></DynamicParameters>
    </Form>
  );
};

export default TemplateForm;
