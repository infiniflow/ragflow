import TopNItem from '@/components/top-n-item';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Select } from 'antd';
import { useMemo } from 'react';
import { Channel } from '../../constant';
import { IOperatorForm } from '../../interface';
import DynamicInputVariable from '../components/dynamic-input-variable';

const DuckDuckGoForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslate('flow');

  const options = useMemo(() => {
    return Object.values(Channel).map((x) => ({ value: x, label: t(x) }));
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
      <TopNItem initialValue={10}></TopNItem>
      <Form.Item
        label={t('channel')}
        name={'channel'}
        tooltip={t('channelTip')}
        initialValue={'text'}
      >
        <Select options={options}></Select>
      </Form.Item>
    </Form>
  );
};

export default DuckDuckGoForm;
