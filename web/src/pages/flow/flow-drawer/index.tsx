import { IModalProps } from '@/interfaces/common';
import { Drawer } from 'antd';
import { Node } from 'reactflow';
import BeginForm from '../begin-form';
import CiteForm from '../cite-form';
import { Operator } from '../constant';
import GenerateForm from '../generate-form';
import RetrievalForm from '../retrieval-form';

interface IProps {
  node?: Node;
}

const FormMap = {
  [Operator.Begin]: BeginForm,
  [Operator.Retrieval]: RetrievalForm,
  [Operator.Generate]: GenerateForm,
  [Operator.Cite]: CiteForm,
};

const FlowDrawer = ({
  visible,
  hideModal,
  node,
}: IModalProps<any> & IProps) => {
  const operatorName: Operator = node?.data.label;
  const OperatorForm = FormMap[operatorName];
  return (
    <Drawer
      title={node?.data.label}
      placement="right"
      onClose={hideModal}
      open={visible}
      getContainer={false}
      mask={false}
      width={500}
    >
      <OperatorForm></OperatorForm>
    </Drawer>
  );
};

export default FlowDrawer;
