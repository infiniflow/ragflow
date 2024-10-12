import KnowledgeBaseItem from '@/components/knowledge-base-item';
import Rerank from '@/components/rerank';
import SimilaritySlider from '@/components/similarity-slider';
import TopNItem from '@/components/top-n-item';
import { useTranslate } from '@/hooks/common-hooks';
import type { FormProps } from 'antd';
import { Form, Input } from 'antd';
import { IOperatorForm } from '../../interface';

type FieldType = {
  top_n?: number;
};

const onFinish: FormProps<FieldType>['onFinish'] = (values) => {
  console.log('Success:', values);
};

const onFinishFailed: FormProps<FieldType>['onFinishFailed'] = (errorInfo) => {
  console.log('Failed:', errorInfo);
};

const RetrievalForm = ({ onValuesChange, form }: IOperatorForm) => {
  const { t } = useTranslate('flow');
  return (
    <Form
      name="basic"
      onFinish={onFinish}
      onFinishFailed={onFinishFailed}
      autoComplete="off"
      onValuesChange={onValuesChange}
      form={form}
      layout={'vertical'}
    >
      <SimilaritySlider
        isTooltipShown
        vectorSimilarityWeightName="keywords_similarity_weight"
      ></SimilaritySlider>
      <TopNItem></TopNItem>
      <Rerank></Rerank>
      <KnowledgeBaseItem></KnowledgeBaseItem>
      <Form.Item
        name={'empty_response'}
        label={t('emptyResponse', { keyPrefix: 'chat' })}
        tooltip={t('emptyResponseTip', { keyPrefix: 'chat' })}
      >
        <Input.TextArea placeholder="" rows={4} />
      </Form.Item>
    </Form>
  );
};

export default RetrievalForm;
