import { ReactComponent as ConfigrationIcon } from '@/assets/svg/knowledge-configration.svg';
import { ReactComponent as DatasetIcon } from '@/assets/svg/knowledge-dataset.svg';
import { ReactComponent as TestingIcon } from '@/assets/svg/knowledge-testing.svg';
import { useSecondPathName } from '@/hooks/routeHook';
import { getWidth } from '@/utils';
import { AntDesignOutlined } from '@ant-design/icons';
import { Avatar, Menu, MenuProps, Space } from 'antd';
import classNames from 'classnames';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useSelector } from 'umi';
import { KnowledgeRouteKey, routeMap } from '../../constant';
import styles from './index.less';

const KnowledgeSidebar = () => {
  const kAModel = useSelector((state: any) => state.kAModel);
  const { id } = kAModel;
  let navigate = useNavigate();
  const activeKey = useSecondPathName();

  const [windowWidth, setWindowWidth] = useState(getWidth());
  const [collapsed, setCollapsed] = useState(false);

  const handleSelect: MenuProps['onSelect'] = (e) => {
    navigate(`/knowledge/${e.key}?id=${id}`);
  };

  type MenuItem = Required<MenuProps>['items'][number];

  const getItem = useCallback(
    (
      label: React.ReactNode,
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
        label,
        type,
        disabled,
      } as MenuItem;
    },
    [],
  );

  const items: MenuItem[] = useMemo(() => {
    return [
      getItem(
        routeMap[KnowledgeRouteKey.Dataset], // TODO: Change icon color when selected
        KnowledgeRouteKey.Dataset,
        <DatasetIcon />,
      ),
      getItem(
        routeMap[KnowledgeRouteKey.Testing],
        KnowledgeRouteKey.Testing,
        <TestingIcon />,
      ),
      getItem(
        routeMap[KnowledgeRouteKey.Configuration],
        KnowledgeRouteKey.Configuration,
        <ConfigrationIcon />,
      ),
      getItem(
        routeMap[KnowledgeRouteKey.TempTesting],
        KnowledgeRouteKey.TempTesting,
        <TestingIcon />,
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

  return (
    <div className={styles.sidebarWrapper}>
      <div className={styles.sidebarTop}>
        <Space size={8} direction="vertical">
          <Avatar size={64} icon={<AntDesignOutlined />} />
          <div className={styles.knowledgeTitle}>Cloud Computing</div>
        </Space>
        <p className={styles.knowledgeDescription}>
          A scalable, secure cloud-based database optimized for high-performance
          computing and data storage.
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
