import React, { useEffect, useState } from 'react';
import { history, Outlet, useLocation, useNavigate } from 'umi';
import { useTranslation, Trans } from 'react-i18next'
import classnames from 'classnames'
import '../locales/config';
import logo from '@/assets/logo.png'
import {
  RedditOutlined
} from '@ant-design/icons';
import { Layout, Button, theme, Space, } from 'antd';
import styles from './index.less'
import User from './components/user'
import { head } from 'lodash';

const { Header, Content } = Layout;

const App: React.FC = (props) => {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const {
    token: { colorBgContainer, borderRadiusLG },
  } = theme.useToken();
  const [current, setCurrent] = useState('knowledge');

  const location = useLocation();
  useEffect(() => {
    if (location.pathname !== '/') {
      const path = location.pathname.split('/')
      setCurrent(path[1]);
    }
    console.log(location.pathname.split('/'))
  }, [location.pathname])

  const handleChange = (path: string) => {
    setCurrent(path)
    navigate(path);
  };
  const tagsData = [{ path: '/knowledge', name: 'knowledge' }, { path: '/chat', name: 'chat' }, { path: '/file', name: 'file' }];

  return (
    <Layout className={styles.layout} >
      <Layout>
        <Header style={{ padding: '0 8px', background: colorBgContainer, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>

          <img src={logo} alt="" style={{ height: 30, width: 30 }} />
          <Space size={[0, 8]} wrap>
            {tagsData.map((item) =>
            (<span key={item.name} className={classnames(styles['tag'], {
              [styles['checked']]: current === item.name
            })} onClick={() => handleChange(item.path)}>
              {item.name}
            </span>)
            )}
          </Space>
          <User ></User>
        </Header>
        <Content
          style={{
            margin: '24px 16px',

            minHeight: 280,
            background: colorBgContainer,
            borderRadius: borderRadiusLG,
            overflow: 'auto'
          }}
        >
          <Outlet />
        </Content>
      </Layout>
    </Layout >
  );
};

export default App;