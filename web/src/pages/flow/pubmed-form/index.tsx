import TopNItem from '@/components/top-n-item';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Input } from 'antd';
import { IOperatorForm } from '../interface';

const PubMedForm = ({ onValuesChange, form }: IOperatorForm) => {
  const { t } = useTranslate('flow');

  return (
    <Form
      name="basic"
      labelCol={{ span: 6 }}
      wrapperCol={{ span: 18 }}
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
    >
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
