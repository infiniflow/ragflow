import { ReactComponent as StarIon } from '@/assets/svg/chat-star.svg';
import { ReactComponent as Logo } from '@/assets/svg/logo.svg';
import { Layout, Radio, Space, theme } from 'antd';

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
        padding: '0 16px',
        background: colorBgContainer,
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        height: '72px',
      }}
    >
      <Space size={12}>
        <Logo className={styles.appIcon}></Logo>
        <label className={styles.appName}>Infinity flow</label>
      </Space>
      <Space size={[0, 8]} wrap>
        <Radio.Group
          defaultValue="a"
          buttonStyle="solid"
          className={styles.radioGroup}
          value={currentPath}
        >
          {tagsData.map((item) => (
            <Radio.Button
              value={item.name}
              onClick={() => handleChange(item.path)}
            >
              <Space>
                <StarIon className={styles.radioButtonIcon}></StarIon>
                {item.name}
              </Space>
            </Radio.Button>
          ))}
        </Radio.Group>
      </Space>
      <User></User>
    </Header>
  );
};

export default RagHeader;
