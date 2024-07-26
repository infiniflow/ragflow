import { useTestChunkRetrieval } from '@/hooks/knowledge-hooks';
import { Flex, Form } from 'antd';
import TestingControl from './testing-control';
import TestingResult from './testing-result';

import styles from './index.less';

const KnowledgeTesting = () => {
  const [form] = Form.useForm();
  const { testChunk } = useTestChunkRetrieval();

  const handleTesting = async (documentIds: string[] = []) => {
    const values = await form.validateFields();
    testChunk({
      ...values,
      doc_ids: Array.isArray(documentIds) ? documentIds : [],
      vector_similarity_weight: 1 - values.vector_similarity_weight,
    });
  };

  return (
    <Flex className={styles.testingWrapper} gap={16}>
      <TestingControl
        form={form}
        handleTesting={handleTesting}
      ></TestingControl>
      <TestingResult handleTesting={handleTesting}></TestingResult>
    </Flex>
  );
};

export default KnowledgeTesting;
