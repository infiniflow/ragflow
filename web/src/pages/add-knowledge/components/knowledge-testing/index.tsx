import { Flex } from 'antd';
import TestingControl from './testing-control';
import TestingResult from './testing-result';

import styles from './index.less';

const KnowledgeTesting = () => {
  return (
    <Flex className={styles.testingWrapper}>
      <TestingControl></TestingControl>
      <TestingResult></TestingResult>
    </Flex>
  );
};

export default KnowledgeTesting;
