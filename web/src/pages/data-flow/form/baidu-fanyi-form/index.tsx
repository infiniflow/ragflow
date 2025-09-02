import { useTranslate } from '@/hooks/common-hooks';
import { Form, Input, Select } from 'antd';
import { useMemo } from 'react';
import { IOperatorForm } from '../../interface';
import {
  BaiduFanyiDomainOptions,
  BaiduFanyiSourceLangOptions,
} from '../../options';
import DynamicInputVariable from '../components/dynamic-input-variable';

const BaiduFanyiForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslate('flow');
  const options = useMemo(() => {
    return ['translate', 'fieldtranslate'].map((x) => ({
      value: x,
      label: t(`baiduSecretKeyOptions.${x}`),
    }));
  }, [t]);

  const baiduFanyiOptions = useMemo(() => {
    return BaiduFanyiDomainOptions.map((x) => ({
      value: x,
      label: t(`baiduDomainOptions.${x}`),
    }));
  }, [t]);

  const baiduFanyiSourceLangOptions = useMemo(() => {
    return BaiduFanyiSourceLangOptions.map((x) => ({
      value: x,
      label: t(`baiduSourceLangOptions.${x}`),
    }));
  }, [t]);

  return (
    <Form
      name="basic"
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
      layout={'vertical'}
    >
      <DynamicInputVariable node={node}></DynamicInputVariable>
      <Form.Item label={t('appid')} name={'appid'}>
        <Input></Input>
      </Form.Item>
      <Form.Item label={t('secretKey')} name={'secret_key'}>
        <Input></Input>
      </Form.Item>
      <Form.Item label={t('transType')} name={'trans_type'}>
        <Select options={options}></Select>
      </Form.Item>
      <Form.Item noStyle dependencies={['model_type']}>
        {({ getFieldValue }) =>
          getFieldValue('trans_type') === 'fieldtranslate' && (
            <Form.Item label={t('domain')} name={'domain'}>
              <Select options={baiduFanyiOptions}></Select>
            </Form.Item>
          )
        }
      </Form.Item>
      <Form.Item label={t('sourceLang')} name={'source_lang'}>
        <Select options={baiduFanyiSourceLangOptions}></Select>
      </Form.Item>
      <Form.Item label={t('targetLang')} name={'target_lang'}>
        <Select options={baiduFanyiSourceLangOptions}></Select>
      </Form.Item>
    </Form>
  );
};

export default BaiduFanyiForm;
