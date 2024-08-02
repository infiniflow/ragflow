import TopNItem from '@/components/top-n-item';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Input, Select } from 'antd';
import { GoogleCountryOptions, GoogleLanguageOptions } from '../constant';
import { IOperatorForm } from '../interface';

const GoogleForm = ({ onValuesChange, form }: IOperatorForm) => {
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
      <Form.Item label={t('apiKey')} name={'api_key'}>
        <Input></Input>
      </Form.Item>
      <Form.Item label={t('country')} name={'country'}>
        <Select options={GoogleCountryOptions}></Select>
      </Form.Item>
      <Form.Item label={t('language')} name={'language'}>
        <Select options={GoogleLanguageOptions}></Select>
      </Form.Item>
    </Form>
  );
};

export default GoogleForm;
