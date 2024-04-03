import { ReactComponent as ConfigurationIcon } from '@/assets/svg/knowledge-configration.svg';
import { ReactComponent as DatasetIcon } from '@/assets/svg/knowledge-dataset.svg';
import { ReactComponent as TestingIcon } from '@/assets/svg/knowledge-testing.svg';
import { useFetchKnowledgeBaseConfiguration } from '@/hooks/knowledgeHook';
import { useSecondPathName } from '@/hooks/routeHook';
import { IKnowledge } from '@/interfaces/database/knowledge';
import { getWidth } from '@/utils';
import { Avatar, Menu, MenuProps, Space } from 'antd';
import classNames from 'classnames';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useSelector } from 'umi';
import { KnowledgeRouteKey } from '../../constant';
import styles from './index.less';

const KnowledgeSidebar = () => {
  const kAModel = useSelector((state: any) => state.kAModel);
  const { id } = kAModel;
  let navigate = useNavigate();
  const activeKey = useSecondPathName();
  const knowledgeDetails: IKnowledge = useSelector(
    (state: any) => state.kSModel.knowledgeDetails,
  );

  const [windowWidth, setWindowWidth] = useState(getWidth());
  const [collapsed, setCollapsed] = useState(false);
  const { t } = useTranslation();

  const handleSelect: MenuProps['onSelect'] = (e) => {
    navigate(`/knowledge/${e.key}?id=${id}`);
  };

  type MenuItem = Required<MenuProps>['items'][number];

  const getItem = useCallback(
    (
      label: string,
      key: React.Key,
      icon?: React.ReactNode,
      disabled?: boolean,
      children?: MenuItem[],
      type?: 'group',
    ): MenuItem => {
      return {
        key,
        icon,
        children,
        label: t(`knowledgeDetails.${label}`),
        type,
        disabled,
      } as MenuItem;
    },
    [t],
  );

  const items: MenuItem[] = useMemo(() => {
    return [
      getItem(
        KnowledgeRouteKey.Dataset, // TODO: Change icon color when selected
        KnowledgeRouteKey.Dataset,
        <DatasetIcon />,
      ),
      getItem(
        KnowledgeRouteKey.Testing,
        KnowledgeRouteKey.Testing,
        <TestingIcon />,
      ),
      getItem(
        KnowledgeRouteKey.Configuration,
        KnowledgeRouteKey.Configuration,
        <ConfigurationIcon />,
      ),
    ];
  }, [getItem]);

  useEffect(() => {
    if (windowWidth.width > 957) {
      setCollapsed(false);
    } else {
      setCollapsed(true);
    }
  }, [windowWidth.width]);

  // 标记一下
  useEffect(() => {
    const widthSize = () => {
      const width = getWidth();

      setWindowWidth(width);
    };
    window.addEventListener('resize', widthSize);
    return () => {
      window.removeEventListener('resize', widthSize);
    };
  }, []);

  useFetchKnowledgeBaseConfiguration();

  return (
    <div className={styles.sidebarWrapper}>
      <div className={styles.sidebarTop}>
        <Space size={8} direction="vertical">
          <Avatar size={64} src={knowledgeDetails.avatar} />
          <div className={styles.knowledgeTitle}>{knowledgeDetails.name}</div>
        </Space>
        <p className={styles.knowledgeDescription}>
          {knowledgeDetails.description}
        </p>
      </div>
      <div className={styles.divider}></div>
      <div className={styles.menuWrapper}>
        <Menu
          selectedKeys={[activeKey]}
          // mode="inline"
          className={classNames(styles.menu, {
            [styles.defaultWidth]: windowWidth.width > 957,
            [styles.minWidth]: windowWidth.width <= 957,
          })}
          // inlineCollapsed={collapsed}
          items={items}
          onSelect={handleSelect}
        />
      </div>
    </div>
  );
};

export default KnowledgeSidebar;
