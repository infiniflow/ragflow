import TopNItem from '@/components/top-n-item';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Select } from 'antd';
import { LanguageOptions } from '../../constant';
import { IOperatorForm } from '../../interface';
import DynamicInputVariable from '../components/dynamic-input-variable';

const WikipediaForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslate('common');

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
      <Form.Item label={t('language')} name={'language'}>
        <Select options={LanguageOptions}></Select>
      </Form.Item>
    </Form>
  );
};

export default WikipediaForm;
