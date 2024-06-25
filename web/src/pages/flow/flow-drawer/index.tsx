import { IModalProps } from '@/interfaces/common';
import { Drawer, Form } from 'antd';
import { useEffect } from 'react';
import { Node } from 'reactflow';
import AnswerForm from '../answer-form';
import BeginForm from '../begin-form';
import CategorizeForm from '../categorize-form';
import { Operator } from '../constant';
import GenerateForm from '../generate-form';
import { useHandleFormValuesChange } from '../hooks';
import RetrievalForm from '../retrieval-form';

interface IProps {
  node?: Node;
}

const FormMap = {
  [Operator.Begin]: BeginForm,
  [Operator.Retrieval]: RetrievalForm,
  [Operator.Generate]: GenerateForm,
  [Operator.Answer]: AnswerForm,
  [Operator.Categorize]: CategorizeForm,
};

const FlowDrawer = ({
  visible,
  hideModal,
  node,
}: IModalProps<any> & IProps) => {
  const operatorName: Operator = node?.data.label;
  const OperatorForm = FormMap[operatorName];
  const [form] = Form.useForm();

  const { handleValuesChange } = useHandleFormValuesChange(node?.id);

  useEffect(() => {
    if (visible) {
      form.setFieldsValue(node?.data?.form);
    }
  }, [visible, form, node?.data?.form]);

  return (
    <Drawer
      title={node?.data.label}
      placement="right"
      onClose={hideModal}
      open={visible}
      getContainer={false}
      mask={false}
      width={470}
    >
      {visible && (
        <OperatorForm
          onValuesChange={handleValuesChange}
          form={form}
        ></OperatorForm>
      )}
    </Drawer>
  );
};

export default FlowDrawer;
