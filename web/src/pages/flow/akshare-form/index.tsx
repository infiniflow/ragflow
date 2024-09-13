import TopNItem from '@/components/top-n-item';
import { Form } from 'antd';
import { IOperatorForm } from '../interface';

const AkShareForm = ({ onValuesChange, form }: IOperatorForm) => {
  return (
    <Form
      name="basic"
      labelCol={{ span: 6 }}
      wrapperCol={{ span: 18 }}
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
    >
      <TopNItem initialValue={10} max={99}></TopNItem>
    </Form>
  );
};

export default AkShareForm;
