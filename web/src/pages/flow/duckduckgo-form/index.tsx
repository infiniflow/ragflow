import TopNItem from '@/components/top-n-item';
import { useTranslate } from '@/hooks/commonHooks';
import { Form, Select } from 'antd';
import { IOperatorForm } from '../interface';

const DuckDuckGoForm = ({ onValuesChange, form }: IOperatorForm) => {
  const { t } = useTranslate('flow');

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
      <Form.Item
        label={t('channel')}
        name={'channel'}
        tooltip={t('channelTip')}
        initialValue={'text'}
      >
        <Select
          options={[
            { value: 'text', label: t('text') },
            { value: 'news', label: t('news') },
          ]}
        ></Select>
      </Form.Item>
    </Form>
  );
};

export default DuckDuckGoForm;
