import { Button, Flex, Space } from 'antd';

import { useSetModalState } from '@/hooks/commonHooks';
import { useFetchFlow } from '@/hooks/flow-hooks';
import { ArrowLeftOutlined } from '@ant-design/icons';
import { Link } from 'umi';
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
  const { data } = useFetchFlow();

  return (
    <>
      <Flex
        align="center"
        justify={'space-between'}
        gap={'large'}
        className={styles.flowHeader}
      >
        <Space size={'large'}>
          <Link to={`/flow`}>
            <ArrowLeftOutlined />
          </Link>
          <h3>{data.title}</h3>
        </Space>
        <Space size={'large'}>
          <Button onClick={showChatDrawer}>
            <b>Debug</b>
          </Button>
          <Button type="primary" onClick={saveGraph}>
            <b>Save</b>
          </Button>
        </Space>
      </Flex>
      <ChatDrawer
        visible={chatDrawerVisible}
        hideModal={hideChatDrawer}
      ></ChatDrawer>
    </>
  );
};

export default FlowHeader;
