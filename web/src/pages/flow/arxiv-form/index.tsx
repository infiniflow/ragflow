import TopNItem from '@/components/top-n-item';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Select } from 'antd';
import { useMemo } from 'react';
import { IOperatorForm } from '../interface';

const ArXivForm = ({ onValuesChange, form }: IOperatorForm) => {
  const { t } = useTranslate('flow');

  const options = useMemo(() => {
    return ['submittedDate', 'lastUpdatedDate', 'relevance'].map((x) => ({
      value: x,
      label: t(x),
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
      <TopNItem initialValue={10}></TopNItem>
      <Form.Item label={t('sortBy')} name={'sort_by'}>
        <Select options={options}></Select>
      </Form.Item>
    </Form>
  );
};

export default ArXivForm;
