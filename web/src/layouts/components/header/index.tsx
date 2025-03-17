import { ReactComponent as HeaderKnowledge } from '@/assets/svg/header-knowledge.svg';
import { ReactComponent as HeaderChat } from '@/assets/svg/header-chat.svg';
import { ReactComponent as HeaderSearch } from '@/assets/svg/header-search.svg';
import { ReactComponent as HeaderAgent } from '@/assets/svg/header-agent.svg';
import { ReactComponent as HeaderFile } from '@/assets/svg/header-file.svg';
import { useTranslate } from '@/hooks/common-hooks';
import { useFetchAppConf } from '@/hooks/logic-hooks';
import { useNavigateWithFromState } from '@/hooks/route-hook';
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
  const tagsData = useMemo(
    () => [
      { path: '/knowledge', name: t('knowledgeBase'), icon: HeaderKnowledge },
      { path: '/chat', name: t('chat'), icon: HeaderChat },
      { path: '/search', name: t('search'), icon: HeaderSearch },
      { path: '/flow', name: t('flow'), icon: HeaderAgent },
      { path: '/file', name: t('fileManager'), icon: HeaderFile },
    ],
    [t],
  );

  const currentPath = useMemo(() => {
    return (
      tagsData.find((x) => pathname.startsWith(x.path))?.name || 'knowledge'
    );
  }, [pathname, tagsData]);

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
        background: 'linear-gradient(90deg, #FFFFFF 0%, #F3F6FD 52.12%, #FFFFFF 100%)',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        height: '54px',
        lineHeight: '46px',
      }}
    >
      <a href={window.location.origin}>
        <Space
          size={12}
          onClick={handleLogoClick}
          className={styles.logoWrapper}
        >
          <img src="/logo-mark.svg" alt="" className={styles.appIcon} />
        </Space>
      </a>
      <Space size={[0, 8]} wrap>
        <Radio.Group
          defaultValue="a"
          buttonStyle="solid"
          className={
            themeRag === 'dark' ? styles.radioGroupDark : styles.radioGroup
          }
          value={currentPath}
        >
          {tagsData.map((item, index) => (
            <Radio.Button
              className={`${themeRag === 'dark' ? 'dark' : 'light'} ${index === 0 ? 'first' : ''} ${index === tagsData.length - 1 ? 'last' : ''}`}
              value={item.name}
              key={item.name}
            >
              <a href={item.path}>
                <Flex
                  align="center"
                  justify='center'
                  gap={8}
                  onClick={handleChange(item.path)}
                  className="cursor-pointer"
                >
                  <item.icon
                    className={styles.radioButtonIcon}
                    stroke={item.name === currentPath ? 'black' : 'white'}
                  ></item.icon>
                  {item.name}
                </Flex>
              </a>
            </Radio.Button>
          ))}
        </Radio.Group>
      </Space>
      <Toolbar></Toolbar>
    </Header>
  );
};

export default RagHeader;
