import {
  useTestChunkAllRetrieval,
  useTestChunkRetrieval,
} from '@/hooks/knowledge-hooks';
import { Flex, Form } from 'antd';
import { useMemo, useState } from 'react';
import TestingControl from './testing-control';
import TestingResult from './testing-result';

import styles from './index.less';

const KnowledgeTesting = () => {
  const [form] = Form.useForm();
  const {
    data: retrievalData,
    testChunk,
    loading: retrievalLoading,
  } = useTestChunkRetrieval();
  const {
    data: allRetrievalData,
    testChunkAll,
    loading: allRetrievalLoading,
  } = useTestChunkAllRetrieval();
  const [selectedDocumentIds, setSelectedDocumentIds] = useState<string[]>([]);

  const handleTesting = async (documentIds: string[] = []) => {
    const values = await form.validateFields();
    const params = {
      ...values,
      vector_similarity_weight: 1 - values.vector_similarity_weight,
    };

    if (Array.isArray(documentIds) && documentIds.length > 0) {
      testChunk({
        ...params,
        doc_ids: documentIds,
      });
    } else {
      testChunkAll({
        ...params,
        doc_ids: [],
      });
    }
  };

  const testingResult = useMemo(() => {
    return selectedDocumentIds.length > 0 ? retrievalData : allRetrievalData;
  }, [allRetrievalData, retrievalData, selectedDocumentIds.length]);

  return (
    <Flex className={styles.testingWrapper} gap={16}>
      <TestingControl
        form={form}
        handleTesting={handleTesting}
        selectedDocumentIds={selectedDocumentIds}
      ></TestingControl>
      <TestingResult
        data={testingResult}
        loading={retrievalLoading || allRetrievalLoading}
        handleTesting={handleTesting}
        selectedDocumentIds={selectedDocumentIds}
        setSelectedDocumentIds={setSelectedDocumentIds}
      ></TestingResult>
    </Flex>
  );
};

export default KnowledgeTesting;
