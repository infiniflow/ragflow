import { useSetModalState } from '@/hooks/common-hooks';
import { ReactFlowProvider } from '@xyflow/react';
import { Layout } from 'antd';
import { useState } from 'react';
import FlowCanvas from './canvas';
import Sider from './flow-sider';
import FlowHeader from './header';
import { useCopyPaste } from './hooks';
import { useFetchDataOnMount } from './hooks/use-fetch-data';

const { Content } = Layout;

function RagFlow() {
  const [collapsed, setCollapsed] = useState(false);
  const {
    visible: chatDrawerVisible,
    hideModal: hideChatDrawer,
    showModal: showChatDrawer,
  } = useSetModalState();

  useFetchDataOnMount();
  useCopyPaste();

  return (
    <Layout>
      <ReactFlowProvider>
        <Sider setCollapsed={setCollapsed} collapsed={collapsed}></Sider>
        <Layout>
          <FlowHeader
            showChatDrawer={showChatDrawer}
            chatDrawerVisible={chatDrawerVisible}
          ></FlowHeader>
          <Content style={{ margin: 0 }}>
            <FlowCanvas
              drawerVisible={chatDrawerVisible}
              hideDrawer={hideChatDrawer}
            ></FlowCanvas>
          </Content>
        </Layout>
      </ReactFlowProvider>
    </Layout>
  );
}

export default RagFlow;
