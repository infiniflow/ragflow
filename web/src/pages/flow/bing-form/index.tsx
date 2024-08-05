import TopNItem from '@/components/top-n-item';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Input, Select } from 'antd';
import { useMemo } from 'react';
import { BingCountryOptions, BingLanguageOptions } from '../constant';
import { IOperatorForm } from '../interface';

const BingForm = ({ onValuesChange, form }: IOperatorForm) => {
  const { t } = useTranslate('flow');

  const options = useMemo(() => {
    return ['Webpages', 'News'].map((x) => ({ label: x, value: x }));
  }, []);

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
      <Form.Item label={t('channel')} name={'channel'}>
        <Select options={options}></Select>
      </Form.Item>
      <Form.Item label={t('apiKey')} name={'api_key'}>
        <Input></Input>
      </Form.Item>
      <Form.Item label={t('country')} name={'country'}>
        <Select options={BingCountryOptions}></Select>
      </Form.Item>
      <Form.Item label={t('language')} name={'language'}>
        <Select options={BingLanguageOptions}></Select>
      </Form.Item>
    </Form>
  );
};

export default BingForm;
