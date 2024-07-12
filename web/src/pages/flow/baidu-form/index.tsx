import TopNItem from '@/components/top-n-item';
import { Form } from 'antd';
import { IOperatorForm } from '../interface';

const BaiduForm = ({ onValuesChange, form }: IOperatorForm) => {
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
    </Form>
  );
};

export default BaiduForm;
