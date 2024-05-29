import { Layout } from 'antd';
import { useState } from 'react';
import { ReactFlowProvider } from 'reactflow';
import FlowCanvas from './canvas';
import Sider from './flow-sider';
import FlowHeader from './header';

const { Content } = Layout;

function RagFlow() {
  const [collapsed, setCollapsed] = useState(false);

  return (
    <Layout>
      <ReactFlowProvider>
        <Sider setCollapsed={setCollapsed} collapsed={collapsed}></Sider>
        <Layout>
          <FlowHeader></FlowHeader>
          <Content style={{ margin: '0 16px' }}>
            <FlowCanvas sideWidth={collapsed ? 0 : 200}></FlowCanvas>
          </Content>
        </Layout>
      </ReactFlowProvider>
    </Layout>
  );
}

export default RagFlow;
