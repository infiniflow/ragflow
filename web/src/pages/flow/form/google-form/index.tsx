import TopNItem from '@/components/top-n-item';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Input, Select } from 'antd';
import { GoogleCountryOptions, GoogleLanguageOptions } from '../../constant';
import { IOperatorForm } from '../../interface';
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
      <Form.Item name={'provider'} label={t('googleprovider')}>
        <Select
          options={[
            { value: 'SerpApi', label: 'SerpApi' },
            { value: 'GoogleCustomSearch', label: 'GoogleCustomSearch' },
            { value: 'OpenSearch', label: 'OpenSearch' },
          ]}
          allowClear={true}
        ></Select>
      </Form.Item>
      <Form.Item label={t('apiKey')} name={'api_key'}>
        <Input placeholder="YOUR_API_KEY (obtained from https://serpapi.com/manage-api-key)"></Input>
      </Form.Item>
      <Form.Item label={t('country')} name={'country'}>
        <Select
          showSearch
          filterOption={(input, option) =>
            (option?.label ?? '').toLowerCase().includes(input.toLowerCase())
          }
          options={GoogleCountryOptions}
        ></Select>
      </Form.Item>
      <Form.Item label={t('language')} name={'language'}>
        <Select
          showSearch
          filterOption={(input, option) =>
            (option?.label ?? '').toLowerCase().includes(input.toLowerCase())
          }
          options={GoogleLanguageOptions}
        ></Select>
      </Form.Item>
    </Form>
  );
};

export default GoogleForm;
