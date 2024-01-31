import logo from '@/assets/logo.png';
import { Layout, Space, theme } from 'antd';
import classnames from 'classnames';

import styles from './index.less';

import { useMemo } from 'react';
import { useLocation, useNavigate } from 'umi';
import User from '../user';

const { Header } = Layout;

const RagHeader = () => {
  const {
    token: { colorBgContainer },
  } = theme.useToken();
  const navigate = useNavigate();
  const { pathname } = useLocation();

  const tagsData = [
    { path: '/knowledge', name: 'knowledge' },
    { path: '/chat', name: 'chat' },
    { path: '/file', name: 'file' },
  ];

  const currentPath = useMemo(() => {
    return tagsData.find((x) => x.path === pathname)?.name || 'knowledge';
  }, [pathname]);

  const handleChange = (path: string) => {
    navigate(path);
  };

  return (
    <Header
      style={{
        padding: '0 8px',
        background: colorBgContainer,
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        height: '72px',
      }}
    >
      <img src={logo} alt="" style={{ height: 30, width: 30 }} />
      <Space size={[0, 8]} wrap>
        {tagsData.map((item) => (
          <span
            key={item.name}
            className={classnames(styles.tag, {
              [styles.checked]: currentPath === item.name,
            })}
            onClick={() => handleChange(item.path)}
          >
            {item.name}
          </span>
        ))}
      </Space>
      <User></User>
    </Header>
  );
};

export default RagHeader;
