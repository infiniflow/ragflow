import TopNItem from '@/components/top-n-item';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Input, Select } from 'antd';
import { IOperatorForm } from '../../interface';
import { GoogleCountryOptions, GoogleLanguageOptions } from '../../options';
import DynamicInputVariable from '../components/dynamic-input-variable';

const GoogleForm = ({ onValuesChange, form, node }: IOperatorForm) => {
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
