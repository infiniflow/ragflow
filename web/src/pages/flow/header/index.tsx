import { Button, Flex } from 'antd';

import { useSaveGraph } from '../hooks';
import styles from './index.less';

const FlowHeader = () => {
  const { saveGraph } = useSaveGraph();

  return (
    <Flex
      align="center"
      justify="end"
      gap={'large'}
      className={styles.flowHeader}
    >
      <Button>
        <b>Debug</b>
      </Button>
      <Button type="primary" onClick={saveGraph}>
        <b>Save</b>
      </Button>
    </Flex>
  );
};

export default FlowHeader;
