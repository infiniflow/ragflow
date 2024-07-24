import TopNItem from '@/components/top-n-item';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Select } from 'antd';
import { LanguageOptions } from '../constant';
import { IOperatorForm } from '../interface';

const WikipediaForm = ({ onValuesChange, form }: IOperatorForm) => {
  const { t } = useTranslate('common');

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
      <Form.Item label={t('language')} name={'language'}>
        <Select options={LanguageOptions}></Select>
      </Form.Item>
    </Form>
  );
};

export default WikipediaForm;
