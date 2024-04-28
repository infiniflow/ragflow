import { Layout } from 'antd';
import FlowCanvas from './canvas';
import Sider from './flow-sider';

const { Content } = Layout;

function RagFlow() {
  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider></Sider>
      <Layout>
        <Content style={{ margin: '0 16px' }}>
          <FlowCanvas></FlowCanvas>
        </Content>
      </Layout>
    </Layout>
  );
}

export default RagFlow;
