import TopNItem from '@/components/top-n-item';
import { Form } from 'antd';
import { IOperatorForm } from '../../interface';

const GithubForm = ({ onValuesChange, form }: IOperatorForm) => {
  return (
    <Form
      name="basic"
      labelCol={{ span: 6 }}
      wrapperCol={{ span: 18 }}
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
    >
      <TopNItem initialValue={5}></TopNItem>
    </Form>
  );
};

export default GithubForm;
