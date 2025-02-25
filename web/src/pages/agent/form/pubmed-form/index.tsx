import TopNItem from '@/components/top-n-item';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Input } from 'antd';
import { IOperatorForm } from '../../interface';
import DynamicInputVariable from '../components/dynamic-input-variable';

const PubMedForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslate('flow');

  return (
    <Form
      name="basic"
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
      layout={'vertical'}
    >
      <DynamicInputVariable node={node}></DynamicInputVariable>
      <TopNItem initialValue={10}></TopNItem>
      <Form.Item
        label={t('email')}
        name={'email'}
        tooltip={t('emailTip')}
        rules={[{ type: 'email' }]}
      >
        <Input></Input>
      </Form.Item>
    </Form>
  );
};

export default PubMedForm;
