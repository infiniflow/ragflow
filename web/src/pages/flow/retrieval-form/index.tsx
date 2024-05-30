import KnowledgeBaseItem from '@/components/knowledge-base-item';
import Rerank from '@/components/rerank';
import SimilaritySlider from '@/components/similarity-slider';
import TopNItem from '@/components/top-n-item';
import type { FormProps } from 'antd';
import { Form } from 'antd';

type FieldType = {
  top_n?: number;
};

const onFinish: FormProps<FieldType>['onFinish'] = (values) => {
  console.log('Success:', values);
};

const onFinishFailed: FormProps<FieldType>['onFinishFailed'] = (errorInfo) => {
  console.log('Failed:', errorInfo);
};

const RetrievalForm = () => {
  return (
    <Form
      name="basic"
      labelCol={{ span: 12 }}
      wrapperCol={{ span: 12 }}
      onFinish={onFinish}
      onFinishFailed={onFinishFailed}
      autoComplete="off"
    >
      <SimilaritySlider isTooltipShown></SimilaritySlider>
      <TopNItem></TopNItem>
      <Rerank></Rerank>
      <KnowledgeBaseItem></KnowledgeBaseItem>
    </Form>
  );
};

export default RetrievalForm;
