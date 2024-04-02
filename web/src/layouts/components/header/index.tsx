import { ReactComponent as StarIon } from '@/assets/svg/chat-star.svg';
import { ReactComponent as KnowledgeBaseIcon } from '@/assets/svg/knowledge-base.svg';
import { ReactComponent as Logo } from '@/assets/svg/logo.svg';
import { Layout, Radio, Space, theme } from 'antd';
import Toolbar from '../right-toolbar';

import styles from './index.less';

import { useNavigateWithFromState } from '@/hooks/routeHook';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation } from 'umi';

const { Header } = Layout;

const RagHeader = () => {
  const {
    token: { colorBgContainer },
  } = theme.useToken();
  const navigate = useNavigateWithFromState();
  const { pathname } = useLocation();
  const { t } = useTranslation('translation', { keyPrefix: 'header' });

  const tagsData = useMemo(
    () => [
      { path: '/knowledge', name: t('knowledgeBase'), icon: KnowledgeBaseIcon },
      { path: '/chat', name: t('chat'), icon: StarIon },
      // { path: '/file', name: 'File Management', icon: FileIcon },
    ],
    [t],
  );

  const currentPath = useMemo(() => {
    return (
      tagsData.find((x) => pathname.startsWith(x.path))?.name || 'knowledge'
    );
  }, [pathname, tagsData]);

  const handleChange = (path: string) => {
    navigate(path);
  };

  const handleLogoClick = useCallback(() => {
    navigate('/');
  }, [navigate]);

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
      <Space size={12} onClick={handleLogoClick} className={styles.logoWrapper}>
        <Logo className={styles.appIcon}></Logo>
        <span className={styles.appName}>RAGFlow</span>
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
              key={item.name}
            >
              <Space>
                <item.icon
                  className={styles.radioButtonIcon}
                  stroke={item.name === currentPath ? 'black' : 'white'}
                ></item.icon>
                {item.name}
              </Space>
            </Radio.Button>
          ))}
        </Radio.Group>
      </Space>
      <Toolbar></Toolbar>
    </Header>
  );
};

export default RagHeader;
