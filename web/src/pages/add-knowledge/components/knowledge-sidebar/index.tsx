import { ReactComponent as ConfigurationIcon } from '@/assets/svg/knowledge-configration.svg';
import { ReactComponent as DatasetIcon } from '@/assets/svg/knowledge-dataset.svg';
import {
  useFetchKnowledgeBaseConfiguration,
  useFetchKnowledgeGraph,
} from '@/hooks/knowledge-hooks';
import {
  useGetKnowledgeSearchParams,
  useSecondPathName,
} from '@/hooks/route-hook';
import { getWidth } from '@/utils';
import { Avatar, Menu, MenuProps, Space } from 'antd';
import classNames from 'classnames';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'umi';
import { KnowledgeRouteKey } from '../../constant';

import { isEmpty } from 'lodash';
import { GitGraph } from 'lucide-react';
import styles from './index.less';

const KnowledgeSidebar = () => {
  const navigate = useNavigate();
  const activeKey = useSecondPathName();
  const { knowledgeId } = useGetKnowledgeSearchParams();

  const [windowWidth, setWindowWidth] = useState(getWidth());
  const [collapsed, setCollapsed] = useState(false);
  const { t } = useTranslation();

  const { data: knowledgeDetails } = useFetchKnowledgeBaseConfiguration();
  const { data } = useFetchKnowledgeGraph();

  /* ---------- helpers ---------- */

  const handleSelect: MenuProps['onSelect'] = (e) => {
    if (e.item?.props?.disabled) return; // ignore greyed items
    navigate(`/knowledge/${e.key}?id=${knowledgeId}`);
  };

  type MenuItem = Required<MenuProps>['items'][number];

  const getItem = useCallback(
    (
      key: React.Key,
      routeKey: KnowledgeRouteKey,
      icon: React.ReactNode,
      disabled = false,
    ): MenuItem => ({
      key: routeKey,
      icon,
      label: t(`knowledgeDetails.${key as string}`),
      disabled,
    }),
    [t],
  );

  /* ---------- menu items ---------- */

  const items: MenuItem[] = useMemo(() => {
    const list: MenuItem[] = [
      getItem(
        KnowledgeRouteKey.Dataset,
        KnowledgeRouteKey.Dataset,
        <DatasetIcon />,
      ),
      getItem(
        KnowledgeRouteKey.Configuration,
        KnowledgeRouteKey.Configuration,
        <ConfigurationIcon />,
        true, // disabled / greyed-out
      ),
    ];

    if (!isEmpty(data?.graph)) {
      list.push(
        getItem(
          KnowledgeRouteKey.KnowledgeGraph,
          KnowledgeRouteKey.KnowledgeGraph,
          <GitGraph />,
        ),
      );
    }

    return list;
  }, [data, getItem]);

  /* ---------- responsive collapse ---------- */

  useEffect(() => {
    setCollapsed(windowWidth.width <= 957);
  }, [windowWidth.width]);

  useEffect(() => {
    const onResize = () => setWindowWidth(getWidth());
    window.addEventListener('resize', onResize);
    return () => window.removeEventListener('resize', onResize);
  }, []);

  /* ---------- render ---------- */

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
          items={items}
          onSelect={handleSelect}
          className={classNames(styles.menu, {
            [styles.defaultWidth]: windowWidth.width > 957,
            [styles.minWidth]: windowWidth.width <= 957,
          })}
          // inlineCollapsed={collapsed}  // keep original comment
        />
      </div>
    </div>
  );
};

export default KnowledgeSidebar;
