import { useTestChunkRetrieval } from '@/hooks/knowledgeHook';
import { Flex, Form } from 'antd';
import { useEffect } from 'react';
import { useDispatch } from 'umi';
import TestingControl from './testing-control';
import TestingResult from './testing-result';

import styles from './index.less';

const KnowledgeTesting = () => {
  const [form] = Form.useForm();
  const testChunk = useTestChunkRetrieval();

  const dispatch = useDispatch();

  const handleTesting = async () => {
    const values = await form.validateFields();
    testChunk(values);
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
