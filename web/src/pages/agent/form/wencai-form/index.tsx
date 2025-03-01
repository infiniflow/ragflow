import TopNItem from '@/components/top-n-item';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Select } from 'antd';
import { useMemo } from 'react';
import { WenCaiQueryTypeOptions } from '../../constant';
import { IOperatorForm } from '../../interface';
import DynamicInputVariable from '../components/dynamic-input-variable';

const WenCaiForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslate('flow');

  const wenCaiQueryTypeOptions = useMemo(() => {
    return WenCaiQueryTypeOptions.map((x) => ({
      value: x,
      label: t(`wenCaiQueryTypeOptions.${x}`),
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
      <TopNItem initialValue={20} max={99}></TopNItem>
      <Form.Item label={t('queryType')} name={'query_type'}>
        <Select options={wenCaiQueryTypeOptions}></Select>
      </Form.Item>
    </Form>
  );
};

export default WenCaiForm;
