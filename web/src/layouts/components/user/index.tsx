import { useFetchUserInfo, useSelectUserInfo } from '@/hooks/userSettingHook';
import { Avatar } from 'antd';
import React from 'react';
import { history } from 'umi';

import styles from '../../index.less';

const App: React.FC = () => {
  const userInfo = useSelectUserInfo();

  const toSetting = () => {
    history.push('/user-setting');
  };

  useFetchUserInfo();

  return (
    <Avatar
      size={32}
      onClick={toSetting}
      className={styles.clickAvailable}
      src={
        userInfo.avatar ??
        'https://zos.alipayobjects.com/rmsportal/jkjgkEfvpUPVyRjUImniVslZfWPnJuuZ.png'
      }
    />
  );
};

export default App;
