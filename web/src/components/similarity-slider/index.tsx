import { Form, Slider } from 'antd';

type FieldType = {
  similarity_threshold?: number;
  vector_similarity_weight?: number;
};

const SimilaritySlider = () => {
  return (
    <>
      <Form.Item<FieldType>
        label="Similarity threshold"
        name={'similarity_threshold'}
        initialValue={0}
      >
        <Slider max={1} step={0.01} />
      </Form.Item>
      <Form.Item<FieldType>
        label="Vector similarity weight"
        name={'vector_similarity_weight'}
        initialValue={0}
      >
        <Slider max={1} step={0.01} />
      </Form.Item>
    </>
  );
};

export default SimilaritySlider;
