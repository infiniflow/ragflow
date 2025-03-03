import TopNItem from '@/components/top-n-item';
import { useTranslate } from '@/hooks/common-hooks';
import { DatePicker, DatePickerProps, Form, Select, Switch } from 'antd';
import dayjs from 'dayjs';
import { useCallback, useMemo } from 'react';
import { useBuildSortOptions } from '../../form-hooks';
import { IOperatorForm } from '../../interface';
import DynamicInputVariable from '../components/dynamic-input-variable';

const YearPicker = ({
  onChange,
  value,
}: {
  onChange?: (val: number | undefined) => void;
  value?: number | undefined;
}) => {
  const handleChange: DatePickerProps['onChange'] = useCallback(
    (val: any) => {
      const nextVal = val?.format('YYYY');
      onChange?.(nextVal ? Number(nextVal) : undefined);
    },
    [onChange],
  );
  // The year needs to be converted into a number and saved to the backend
  const nextValue = useMemo(() => {
    if (value) {
      return dayjs(value.toString());
    }
    return undefined;
  }, [value]);

  return <DatePicker picker="year" onChange={handleChange} value={nextValue} />;
};

const GoogleScholarForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslate('flow');

  const options = useBuildSortOptions();

  return (
    <Form
      name="basic"
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
      layout={'vertical'}
    >
      <DynamicInputVariable node={node}></DynamicInputVariable>
      <TopNItem initialValue={5}></TopNItem>
      <Form.Item
        label={t('sortBy')}
        name={'sort_by'}
        initialValue={'relevance'}
      >
        <Select options={options}></Select>
      </Form.Item>
      <Form.Item label={t('yearLow')} name={'year_low'}>
        <YearPicker />
      </Form.Item>
      <Form.Item label={t('yearHigh')} name={'year_high'}>
        <YearPicker />
      </Form.Item>
      <Form.Item
        label={t('patents')}
        name={'patents'}
        valuePropName="checked"
        initialValue={true}
      >
        <Switch></Switch>
      </Form.Item>
    </Form>
  );
};

export default GoogleScholarForm;
