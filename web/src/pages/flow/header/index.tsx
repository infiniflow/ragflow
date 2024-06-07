import { Button, Flex } from 'antd';

import { useSetModalState } from '@/hooks/commonHooks';
import ChatDrawer from '../chat/drawer';
import { useRunGraph, useSaveGraph } from '../hooks';
import styles from './index.less';

const FlowHeader = () => {
  const { saveGraph } = useSaveGraph();
  const { runGraph } = useRunGraph();
  const {
    visible: chatDrawerVisible,
    hideModal: hideChatDrawer,
    showModal: showChatDrawer,
  } = useSetModalState();

  return (
    <>
      <Flex
        align="center"
        justify="end"
        gap={'large'}
        className={styles.flowHeader}
      >
        <Button onClick={showChatDrawer}>
          <b>Debug</b>
        </Button>
        <Button type="primary" onClick={saveGraph}>
          <b>Save</b>
        </Button>
      </Flex>
      <ChatDrawer
        visible={chatDrawerVisible}
        hideModal={hideChatDrawer}
      ></ChatDrawer>
    </>
  );
};

export default FlowHeader;
