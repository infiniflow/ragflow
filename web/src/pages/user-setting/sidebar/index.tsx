import { useSecondPathName } from '@/hooks/routeHook';
import type { MenuProps } from 'antd';
import { Menu } from 'antd';
import React, { useMemo } from 'react';
import { useNavigate } from 'umi';
import {
  UserSettingBaseKey,
  UserSettingIconMap,
  UserSettingRouteKey,
  UserSettingRouteMap,
} from '../constants';

import { useLogout } from '@/hooks/userSettingHook';
import styles from './index.less';

type MenuItem = Required<MenuProps>['items'][number];

function getItem(
  label: React.ReactNode,
  key: React.Key,
  icon?: React.ReactNode,
  children?: MenuItem[],
  type?: 'group',
): MenuItem {
  return {
    key,
    icon,
    children,
    label,
    type,
  } as MenuItem;
}

const items: MenuItem[] = Object.values(UserSettingRouteKey).map((value) =>
  getItem(UserSettingRouteMap[value], value, UserSettingIconMap[value]),
);

const SideBar = () => {
  const navigate = useNavigate();
  const pathName = useSecondPathName();
  const logout = useLogout();

  const handleMenuClick: MenuProps['onClick'] = ({ key }) => {
    if (key === UserSettingRouteKey.Logout) {
      logout();
    } else {
      navigate(`/${UserSettingBaseKey}/${key}`);
    }
  };

  const selectedKeys = useMemo(() => {
    return [pathName];
  }, [pathName]);

  return (
    <section className={styles.sideBarWrapper}>
      <Menu
        selectedKeys={selectedKeys}
        mode="inline"
        items={items}
        onClick={handleMenuClick}
        style={{ width: 312 }}
      />
    </section>
  );
};

export default SideBar;
