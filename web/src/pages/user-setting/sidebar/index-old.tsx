import { useIsDarkTheme, useTheme } from '@/components/theme-provider';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Domain } from '@/constants/common';
import { useTranslate } from '@/hooks/common-hooks';
import { useLogout } from '@/hooks/login-hooks';
import { useSecondPathName } from '@/hooks/route-hook';
import { useFetchSystemVersion } from '@/hooks/user-setting-hooks';
import type { MenuProps } from 'antd';
import { Flex, Menu } from 'antd';
import { LogOut } from 'lucide-react';
import React, { useCallback, useEffect, useMemo } from 'react';
import { useNavigate } from 'umi';
import {
  UserSettingBaseKey,
  UserSettingIconMap,
  UserSettingRouteKey,
} from '../constants';
import styles from './index.less';

type MenuItem = Required<MenuProps>['items'][number];

const SideBar = () => {
  const navigate = useNavigate();
  const pathName = useSecondPathName();
  const { logout } = useLogout();
  const { t } = useTranslate('setting');
  const { version, fetchSystemVersion } = useFetchSystemVersion();

  const { setTheme } = useTheme();
  const isDarkTheme = useIsDarkTheme();

  const handleThemeChange = useCallback(
    (checked: boolean) => {
      setTheme(checked ? 'dark' : 'light');
    },
    [setTheme],
  );

  useEffect(() => {
    if (location.host !== Domain) {
      fetchSystemVersion();
    }
  }, [fetchSystemVersion]);

  function getItem(
    label: string,
    key: React.Key,
    icon?: React.ReactNode,
    children?: MenuItem[],
    type?: 'group',
  ): MenuItem {
    return {
      key,
      icon,
      children,
      label: (
        <Flex justify={'space-between'}>
          {t(label)}
          <span className={styles.version}>
            {label === 'system' && version}
          </span>
        </Flex>
      ),
      type,
    } as MenuItem;
  }

  const items: MenuItem[] = Object.values(UserSettingRouteKey).map((value) =>
    getItem(value, value, UserSettingIconMap[value]),
  );

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
    <section className={`${styles.sideBarWrapper} flex flex-col border-r`}>
      <Menu
        selectedKeys={selectedKeys}
        mode="inline"
        items={items}
        onClick={handleMenuClick}
        style={{ width: 312 }}
      />
      <div className="p-6 mt-auto border-t">
        <div className="flex items-center gap-2 mb-6">
          <Switch
            id="dark-mode"
            onCheckedChange={handleThemeChange}
            checked={isDarkTheme}
          />
          <Label htmlFor="dark-mode" className="text-sm">
            Dark
          </Label>
        </div>
        <Button
          variant="outline"
          className="w-full gap-3"
          onClick={() => {
            logout();
          }}
        >
          <LogOut className="w-6 h-6" />
          Logout
        </Button>
      </div>
    </section>
  );
};

export default SideBar;
