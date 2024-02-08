import { Flex, Form } from 'antd';
import TestingControl from './testing-control';
import TestingResult from './testing-result';

import { useKnowledgeBaseId } from '@/hooks/knowledgeHook';
import { useEffect } from 'react';
import { useDispatch } from 'umi';
import styles from './index.less';

const KnowledgeTesting = () => {
  const [form] = Form.useForm();

  const dispatch = useDispatch();
  const knowledgeBaseId = useKnowledgeBaseId();

  const handleTesting = async () => {
    const values = await form.validateFields();
    console.info(values);
    const similarity_threshold = values.similarity_threshold / 100;
    const vector_similarity_weight = values.vector_similarity_weight / 100;
    dispatch({
      type: 'testingModel/testDocumentChunk',
      payload: {
        ...values,
        similarity_threshold,
        vector_similarity_weight,
        kb_id: knowledgeBaseId,
      },
    });
  };

  useEffect(() => {
    return () => {
      dispatch({ type: 'testingModel/reset' });
    };
  }, [dispatch]);

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
