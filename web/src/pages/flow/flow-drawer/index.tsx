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
import MessageForm from '../message-form';
import RelevantForm from '../relevant-form';
import RetrievalForm from '../retrieval-form';
import RewriteQuestionForm from '../rewrite-question-form';

interface IProps {
  node?: Node;
}

const FormMap = {
  [Operator.Begin]: BeginForm,
  [Operator.Retrieval]: RetrievalForm,
  [Operator.Generate]: GenerateForm,
  [Operator.Answer]: AnswerForm,
  [Operator.Categorize]: CategorizeForm,
  [Operator.Message]: MessageForm,
  [Operator.Relevant]: RelevantForm,
  [Operator.RewriteQuestion]: RewriteQuestionForm,
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
          node={node}
        ></OperatorForm>
      )}
    </Drawer>
  );
};

export default FlowDrawer;
