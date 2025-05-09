import { ReactComponent as FileIcon } from '@/assets/svg/file-management.svg';
import { ReactComponent as GraphIcon } from '@/assets/svg/graph.svg';
import { ReactComponent as KnowledgeBaseIcon } from '@/assets/svg/knowledge-base.svg';
import { useTranslate } from '@/hooks/common-hooks';
import { useFetchAppConf } from '@/hooks/logic-hooks';
import { useNavigateWithFromState } from '@/hooks/route-hook';
import { MessageOutlined, SearchOutlined } from '@ant-design/icons';
import { Flex, Layout, Radio, Space, theme } from 'antd';
import { MouseEventHandler, useCallback, useMemo } from 'react';
import { useLocation } from 'umi';
import Toolbar from '../right-toolbar';

import { useTheme } from '@/components/theme-provider';
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
  const { theme: themeRag } = useTheme();

  /** navigation definitions */
  const tagsData = useMemo(
    () => [
      { path: '/knowledge', name: t('knowledgeBase'), icon: KnowledgeBaseIcon },
      { path: '/chat', name: t('chat'), icon: MessageOutlined },
      {
        path: '/search',
        name: t('search'),
        icon: SearchOutlined,
        disabled: true,
      }, // 🔒 disabled
      { path: '/flow', name: t('flow'), icon: GraphIcon, disabled: true },
      { path: '/file', name: t('fileManager'), icon: FileIcon, disabled: true },
    ],
    [t],
  );

  const currentPath = useMemo(() => {
    return (
      tagsData.find((x) => pathname.startsWith(x.path))?.name ||
      t('knowledgeBase')
    );
  }, [pathname, tagsData, t]);

  /** click handler */
  const handleChange = useCallback(
    (path: string): MouseEventHandler =>
      (e) => {
        e.preventDefault();
        navigate(path);
      },
    [navigate],
  );

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
      <a href={window.location.origin}>
        <Space
          size={12}
          onClick={handleLogoClick}
          className={styles.logoWrapper}
        >
          <img src="/logo.svg" alt="" className={styles.appIcon} />
          <span className={styles.appName}>{appConf.appName}</span>
        </Space>
      </a>

      {/* NAV TABS */}
      <Space size={[0, 8]} wrap>
        <Radio.Group
          buttonStyle="solid"
          className={
            themeRag === 'dark' ? styles.radioGroupDark : styles.radioGroup
          }
          value={currentPath}
        >
          {tagsData.map((item, index) => (
            <Radio.Button
              key={item.name}
              value={item.name}
              disabled={item.disabled}
              className={`${themeRag === 'dark' ? 'dark' : 'light'} ${
                index === 0 ? 'first' : ''
              } ${index === tagsData.length - 1 ? 'last' : ''}`}
            >
              <Flex
                align="center"
                gap={8}
                onClick={item.disabled ? undefined : handleChange(item.path)}
                style={
                  item.disabled
                    ? { cursor: 'not-allowed', opacity: 0.4 }
                    : undefined
                }
              >
                <item.icon
                  className={styles.radioButtonIcon}
                  stroke={item.name === currentPath ? 'black' : 'white'}
                />
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
