import {
  useFetchUserInfo,
  useLogout,
  useSelectUserInfo,
} from '@/hooks/userSettingHook';
import authorizationUtil from '@/utils/authorizationUtil';
import type { MenuProps } from 'antd';
import { Avatar, Button, Dropdown } from 'antd';
import React, { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { history } from 'umi';

const App: React.FC = () => {
  const { t } = useTranslation();
  const userInfo = useSelectUserInfo();
  const logout = useLogout();

  const handleLogout = useCallback(async () => {
    const retcode = await logout();
    if (retcode === 0) {
      authorizationUtil.removeAll();
      history.push('/login');
    }
  }, [logout]);

  const toSetting = () => {
    history.push('/user-setting');
  };

  const items: MenuProps['items'] = useMemo(() => {
    return [
      {
        key: '1',
        onClick: handleLogout,
        label: <Button type="text">{t('header.logout')}</Button>,
      },
      {
        key: '2',
        onClick: toSetting,
        label: <Button type="text">{t('header.setting')}</Button>,
      },
    ];
  }, [t, handleLogout]);

  useFetchUserInfo();

  return (
    <Dropdown menu={{ items }} placement="bottomLeft" arrow>
      <Avatar
        size={32}
        src={
          userInfo.avatar ??
          'https://zos.alipayobjects.com/rmsportal/jkjgkEfvpUPVyRjUImniVslZfWPnJuuZ.png'
        }
      />
    </Dropdown>
  );
};

export default App;
