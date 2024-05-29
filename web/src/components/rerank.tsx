import { LlmModelType } from '@/constants/knowledge';
import { useTranslate } from '@/hooks/commonHooks';
import { useSelectLlmOptionsByModelType } from '@/hooks/llmHooks';
import { Form, Select, Slider } from 'antd';

type FieldType = {
  rerank_id?: string;
  top_k?: number;
};

export const RerankItem = () => {
  const { t } = useTranslate('knowledgeDetails');
  const allOptions = useSelectLlmOptionsByModelType();

  return (
    <Form.Item
      label={t('rerankModel')}
      name={'rerank_id'}
      tooltip={t('rerankTip')}
    >
      <Select
        options={allOptions[LlmModelType.Rerank]}
        allowClear
        placeholder={t('rerankPlaceholder')}
      />
    </Form.Item>
  );
};

const Rerank = () => {
  const { t } = useTranslate('knowledgeDetails');

  return (
    <>
      <RerankItem></RerankItem>
      <Form.Item noStyle dependencies={['rerank_id']}>
        {({ getFieldValue }) => {
          const rerankId = getFieldValue('rerank_id');
          return (
            rerankId && (
              <Form.Item<FieldType>
                label={t('topK')}
                name={'top_k'}
                initialValue={1024}
                tooltip={t('topKTip')}
              >
                <Slider max={2048} min={1} />
              </Form.Item>
            )
          );
        }}
      </Form.Item>
    </>
  );
};

export default Rerank;
