import { Button, Flex } from 'antd';

import { useRunGraph, useSaveGraph } from '../hooks';
import styles from './index.less';

const FlowHeader = () => {
  const { saveGraph } = useSaveGraph();
  const { runGraph } = useRunGraph();

  return (
    <Flex
      align="center"
      justify="end"
      gap={'large'}
      className={styles.flowHeader}
    >
      <Button onClick={runGraph}>
        <b>Debug</b>
      </Button>
      <Button type="primary" onClick={saveGraph}>
        <b>Save</b>
      </Button>
    </Flex>
  );
};

export default FlowHeader;
