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
    dispatch({
      type: 'testingModel/testDocumentChunk',
      payload: {
        ...values,
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
