import { IModalProps } from '@/interfaces/common';
import { Drawer } from 'antd';
import { Node } from 'reactflow';
import AnswerForm from '../answer-form';
import BeginForm from '../begin-form';
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
};

const FlowDrawer = ({
  visible,
  hideModal,
  node,
}: IModalProps<any> & IProps) => {
  const operatorName: Operator = node?.data.label;
  const OperatorForm = FormMap[operatorName];
  const { handleValuesChange } = useHandleFormValuesChange(node?.id);

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
        <OperatorForm onValuesChange={handleValuesChange}></OperatorForm>
      )}
    </Drawer>
  );
};

export default FlowDrawer;
