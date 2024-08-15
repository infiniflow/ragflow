import { Divider, Layout, theme } from 'antd';
import React from 'react';
import { Outlet } from 'umi';
import '../locales/config';
import Header from './components/header';

import styles from './index.less';

const { Content } = Layout;

const App: React.FC = () => {
  const {
    token: { colorBgContainer, borderRadiusLG },
  } = theme.useToken();

  return (
    <Layout className={styles.layout}>
      <Layout>
        <Header></Header>
        <Divider orientationMargin={0} className={styles.divider} />
        <Content
          style={{
            minHeight: 280,
            background: colorBgContainer,
            borderRadius: borderRadiusLG,
            overflow: 'auto',
            display: 'flex',
          }}
        >
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  );
};

export default App;
