import {
  useTestChunkAllRetrieval,
  useTestChunkRetrieval,
} from '@/hooks/knowledge-hooks';
import { Flex, Form } from 'antd';
import TestingControl from './testing-control';
import TestingResult from './testing-result';

import { useState } from 'react';
import styles from './index.less';

const KnowledgeTesting = () => {
  const [form] = Form.useForm();
  const { testChunk } = useTestChunkRetrieval();
  const { testChunkAll } = useTestChunkAllRetrieval();
  const [selectedDocumentIds, setSelectedDocumentIds] = useState<string[]>([]);

  const handleTesting = async (documentIds: string[] = []) => {
    const values = await form.validateFields();
    testChunk({
      ...values,
      doc_ids: Array.isArray(documentIds) ? documentIds : [],
      vector_similarity_weight: 1 - values.vector_similarity_weight,
    });

    testChunkAll({
      ...values,
      doc_ids: [],
      vector_similarity_weight: 1 - values.vector_similarity_weight,
    });
  };

  return (
    <Flex className={styles.testingWrapper} gap={16}>
      <TestingControl
        form={form}
        handleTesting={handleTesting}
        selectedDocumentIds={selectedDocumentIds}
      ></TestingControl>
      <TestingResult
        handleTesting={handleTesting}
        selectedDocumentIds={selectedDocumentIds}
        setSelectedDocumentIds={setSelectedDocumentIds}
      ></TestingResult>
    </Flex>
  );
};

export default KnowledgeTesting;
