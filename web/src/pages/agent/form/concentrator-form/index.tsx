import { Form } from 'antd';
import { IOperatorForm } from '../../interface';

const ConcentratorForm = ({ onValuesChange, form }: IOperatorForm) => {
  return (
    <Form
      name="basic"
      labelCol={{ span: 8 }}
      wrapperCol={{ span: 16 }}
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
    ></Form>
  );
};

export default ConcentratorForm;
