import { useFetchUserInfo, useSelectUserInfo } from '@/hooks/userSettingHook';
import authorizationUtil from '@/utils/authorizationUtil';
import type { MenuProps } from 'antd';
import { Avatar, Button, Dropdown } from 'antd';
import React, { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { history } from 'umi';

const App: React.FC = () => {
  const { t } = useTranslation();
  const userInfo = useSelectUserInfo();

  const logout = () => {
    authorizationUtil.removeAll();
    history.push('/login');
  };

  const toSetting = () => {
    history.push('/setting');
  };

  const items: MenuProps['items'] = useMemo(() => {
    return [
      {
        key: '1',
        onClick: logout,
        label: <Button type="text">{t('header.logout')}</Button>,
      },
      {
        key: '2',
        onClick: toSetting,
        label: <Button type="text">{t('header.setting')}</Button>,
      },
    ];
  }, [t]);

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
