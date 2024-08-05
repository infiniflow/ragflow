import { ReactComponent as FileIcon } from '@/assets/svg/file-management.svg';
import { ReactComponent as GraphIcon } from '@/assets/svg/graph.svg';
import { ReactComponent as KnowledgeBaseIcon } from '@/assets/svg/knowledge-base.svg';
import { useTranslate } from '@/hooks/common-hooks';
import { useNavigateWithFromState } from '@/hooks/route-hook';
import { Flex, Layout, Radio, Space, theme } from 'antd';
import { useCallback, useMemo } from 'react';
import { useLocation } from 'umi';
import Toolbar from '../right-toolbar';

import { useFetchAppConf } from '@/hooks/logic-hooks';
import { MessageOutlined } from '@ant-design/icons';
import styles from './index.less';

const { Header } = Layout;

const RagHeader = () => {
  const {
    token: { colorBgContainer },
  } = theme.useToken();
  const navigate = useNavigateWithFromState();
  const { pathname } = useLocation();
  const { t } = useTranslate('header');
  const appConf = useFetchAppConf();

  const tagsData = useMemo(
    () => [
      { path: '/knowledge', name: t('knowledgeBase'), icon: KnowledgeBaseIcon },
      { path: '/chat', name: t('chat'), icon: MessageOutlined },
      { path: '/flow', name: t('flow'), icon: GraphIcon },
      { path: '/file', name: t('fileManager'), icon: FileIcon },
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
        <img src="/logo.svg" alt="" className={styles.appIcon} />
        <span className={styles.appName}>{appConf.appName}</span>
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
              <Flex align="center" gap={8}>
                <item.icon
                  className={styles.radioButtonIcon}
                  stroke={item.name === currentPath ? 'black' : 'white'}
                ></item.icon>
                {item.name}
              </Flex>
            </Radio.Button>
          ))}
        </Radio.Group>
      </Space>
      <Toolbar></Toolbar>
    </Header>
  );
};

export default RagHeader;
