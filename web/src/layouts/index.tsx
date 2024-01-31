import { Divider, Layout, theme } from 'antd';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { Outlet } from 'umi';
import '../locales/config';
import Header from './components/header';
import styles from './index.less';

const { Content } = Layout;

const App: React.FC = () => {
  const { t } = useTranslation();
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
          }}
        >
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  );
};

export default App;
