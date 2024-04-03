import { useTranslate } from '@/hooks/commonHooks';
import { Form, Slider } from 'antd';

type FieldType = {
  similarity_threshold?: number;
  vector_similarity_weight?: number;
};

interface IProps {
  isTooltipShown?: boolean;
}

const SimilaritySlider = ({ isTooltipShown = false }: IProps) => {
  const { t } = useTranslate('knowledgeDetails');

  return (
    <>
      <Form.Item<FieldType>
        label={t('similarityThreshold')}
        name={'similarity_threshold'}
        tooltip={isTooltipShown && t('similarityThresholdTip')}
        initialValue={0.2}
      >
        <Slider max={1} step={0.01} />
      </Form.Item>
      <Form.Item<FieldType>
        label={t('vectorSimilarityWeight')}
        name={'vector_similarity_weight'}
        initialValue={0.3}
        tooltip={isTooltipShown && t('vectorSimilarityWeightTip')}
      >
        <Slider max={1} step={0.01} />
      </Form.Item>
    </>
  );
};

export default SimilaritySlider;
