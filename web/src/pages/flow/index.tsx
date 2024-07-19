import { useSetModalState } from '@/hooks/common-hooks';
import { Layout } from 'antd';
import { useState } from 'react';
import { ReactFlowProvider } from 'reactflow';
import FlowCanvas from './canvas';
import Sider from './flow-sider';
import FlowHeader from './header';
import { useFetchDataOnMount } from './hooks';

const { Content } = Layout;

function RagFlow() {
  const [collapsed, setCollapsed] = useState(false);
  const {
    visible: chatDrawerVisible,
    hideModal: hideChatDrawer,
    showModal: showChatDrawer,
  } = useSetModalState();

  useFetchDataOnMount();

  return (
    <Layout>
      <ReactFlowProvider>
        <Sider setCollapsed={setCollapsed} collapsed={collapsed}></Sider>
        <Layout>
          <FlowHeader showChatDrawer={showChatDrawer}></FlowHeader>
          <Content style={{ margin: 0 }}>
            <FlowCanvas
              chatDrawerVisible={chatDrawerVisible}
              hideChatDrawer={hideChatDrawer}
            ></FlowCanvas>
          </Content>
        </Layout>
      </ReactFlowProvider>
    </Layout>
  );
}

export default RagFlow;
