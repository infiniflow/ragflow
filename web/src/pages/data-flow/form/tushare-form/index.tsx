import { useTranslate } from '@/hooks/common-hooks';
import { DatePicker, DatePickerProps, Form, Input, Select } from 'antd';
import dayjs from 'dayjs';
import { useCallback, useMemo } from 'react';
import { IOperatorForm } from '../../interface';
import { TuShareSrcOptions } from '../../options';
import DynamicInputVariable from '../components/dynamic-input-variable';

const DateTimePicker = ({
  onChange,
  value,
}: {
  onChange?: (val: number | undefined) => void;
  value?: number | undefined;
}) => {
  const handleChange: DatePickerProps['onChange'] = useCallback(
    (val: any) => {
      const nextVal = val?.format('YYYY-MM-DD HH:mm:ss');
      onChange?.(nextVal ? nextVal : undefined);
    },
    [onChange],
  );
  // The value needs to be converted into a string and saved to the backend
  const nextValue = useMemo(() => {
    if (value) {
      return dayjs(value);
    }
    return undefined;
  }, [value]);

  return (
    <DatePicker
      showTime
      format="YYYY-MM-DD HH:mm:ss"
      onChange={handleChange}
      value={nextValue}
    />
  );
};

const TuShareForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslate('flow');

  const tuShareSrcOptions = useMemo(() => {
    return TuShareSrcOptions.map((x) => ({
      value: x,
      label: t(`tuShareSrcOptions.${x}`),
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
      <Form.Item
        label={t('token')}
        name={'token'}
        tooltip={'Get from https://tushare.pro/'}
      >
        <Input></Input>
      </Form.Item>
      <Form.Item label={t('src')} name={'src'}>
        <Select options={tuShareSrcOptions}></Select>
      </Form.Item>
      <Form.Item label={t('startDate')} name={'start_date'}>
        <DateTimePicker />
      </Form.Item>
      <Form.Item label={t('endDate')} name={'end_date'}>
        <DateTimePicker />
      </Form.Item>
      <Form.Item label={t('keyword')} name={'keyword'}>
        <Input></Input>
      </Form.Item>
    </Form>
  );
};

export default TuShareForm;
