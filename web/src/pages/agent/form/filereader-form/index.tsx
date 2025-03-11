import { Form } from 'antd';
import { IOperatorForm } from '../../interface';
import DynamicInputVariable from '../components/dynamic-input-variable';

const FileReaderForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  return (
    <Form
      name="basic"
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
      layout={'vertical'}
    >
      <DynamicInputVariable node={node}></DynamicInputVariable>
    </Form>
  );
};

export default FileReaderForm;
