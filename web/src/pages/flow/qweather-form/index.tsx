import { useTranslate } from '@/hooks/common-hooks';
import { Form, Input, Select } from 'antd';
import { useMemo } from 'react';
import {
  QWeatherLangOptions,
  QWeatherTimePeriodOptions,
  QWeatherTypeOptions,
  QWeatherUserTypeOptions,
} from '../constant';
import { IOperatorForm } from '../interface';

const QWeatherForm = ({ onValuesChange, form }: IOperatorForm) => {
  const { t } = useTranslate('flow');
  const qWeatherLangOptions = useMemo(() => {
    return QWeatherLangOptions.map((x) => ({
      value: x,
      label: t(`qWeatherLangOptions.${x}`),
    }));
  }, [t]);

  const qWeatherTypeOptions = useMemo(() => {
    return QWeatherTypeOptions.map((x) => ({
      value: x,
      label: t(`qWeatherTypeOptions.${x}`),
    }));
  }, [t]);

  const qWeatherUserTypeOptions = useMemo(() => {
    return QWeatherUserTypeOptions.map((x) => ({
      value: x,
      label: t(`qWeatherUserTypeOptions.${x}`),
    }));
  }, [t]);

  const qWeatherTimePeriodOptions = useMemo(() => {
    return QWeatherTimePeriodOptions.map((x) => ({
      value: x,
      label: t(`qWeatherTimePeriodOptions.${x}`),
    }));
  }, [t]);

  return (
    <Form
      name="basic"
      labelCol={{ span: 6 }}
      wrapperCol={{ span: 18 }}
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
    >
      <Form.Item label={t('webApiKey')} name={'web_apikey'}>
        <Input></Input>
      </Form.Item>
      <Form.Item label={t('lang')} name={'lang'}>
        <Select options={qWeatherLangOptions}></Select>
      </Form.Item>
      <Form.Item label={t('type')} name={'type'}>
        <Select options={qWeatherTypeOptions}></Select>
      </Form.Item>
      <Form.Item label={t('userType')} name={'user_type'}>
        <Select options={qWeatherUserTypeOptions}></Select>
      </Form.Item>
      <Form.Item noStyle dependencies={['type']}>
        {({ getFieldValue }) =>
          getFieldValue('type') === 'weather' && (
            <Form.Item label={t('timePeriod')} name={'time_period'}>
              <Select options={qWeatherTimePeriodOptions}></Select>
            </Form.Item>
          )
        }
      </Form.Item>
    </Form>
  );
};

export default QWeatherForm;
