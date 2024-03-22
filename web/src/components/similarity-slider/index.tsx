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
        tooltip={isTooltipShown && `We use hybrid similarity score to evaluate distance between two lines of text. 
        It\'s weighted keywords similarity and vector cosine similarity. 
        If the similarity between query and chunk is less than this threshold, the chunk will be filtered out.`
    }
        initialValue={0.2}
      >
        <Slider max={1} step={0.01} />
      </Form.Item>
      <Form.Item<FieldType>
        label="Vector similarity weight"
        name={'vector_similarity_weight'}
        initialValue={0.3}
        tooltip={isTooltipShown && `We use hybrid similarity score to evaluate distance between two lines of text. 
        It\'s weighted keywords similarity and vector cosine similarity.
        The sum of both weights is 1.0.
        `}
      >
        <Slider max={1} step={0.01} />
      </Form.Item>
    </>
  );
};

export default SimilaritySlider;
