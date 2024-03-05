import { Form, Slider } from 'antd';

type FieldType = {
  similarity_threshold?: number;
  vector_similarity_weight?: number;
};

interface IProps {
  isTooltipShown?: boolean;
}

const SimilaritySlider = ({ isTooltipShown = false }: IProps) => {
  return (
    <>
      <Form.Item<FieldType>
        label="Similarity threshold"
        name={'similarity_threshold'}
        tooltip={isTooltipShown && 'xxx'}
        initialValue={0.2}
      >
        <Slider max={1} step={0.01} />
      </Form.Item>
      <Form.Item<FieldType>
        label="Vector similarity weight"
        name={'vector_similarity_weight'}
        initialValue={0.3}
        tooltip={isTooltipShown && 'xxx'}
      >
        <Slider max={1} step={0.01} />
      </Form.Item>
    </>
  );
};

export default SimilaritySlider;
