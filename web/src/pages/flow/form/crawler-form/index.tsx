import { useTranslate } from '@/hooks/common-hooks';
import { Form, Input, Select } from 'antd';
import { useMemo } from 'react';
import { CrawlerResultOptions } from '../../constant';
import { IOperatorForm } from '../../interface';
import DynamicInputVariable from '../components/dynamic-input-variable';
const CrawlerForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslate('flow');
  const crawlerResultOptions = useMemo(() => {
    return CrawlerResultOptions.map((x) => ({
      value: x,
      label: t(`crawlerResultOptions.${x}`),
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
      <Form.Item label={t('proxy')} name={'proxy'}>
        <Input placeholder="like: http://127.0.0.1:8888"></Input>
      </Form.Item>
      <Form.Item
        label={t('extractType')}
        name={'extract_type'}
        initialValue="markdown"
      >
        <Select options={crawlerResultOptions}></Select>
      </Form.Item>
    </Form>
  );
};

export default CrawlerForm;
