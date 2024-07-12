import TopNItem from '@/components/top-n-item';
import { useTranslate } from '@/hooks/commonHooks';
import { Form, Input } from 'antd';
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
      <TopNItem></TopNItem>
      <Form.Item
        label={t('channel')}
        name={'channel'}
        tooltip={t('channelTip')}
      >
        <Input.TextArea rows={5} />
      </Form.Item>
    </Form>
  );
};

export default DuckDuckGoForm;
