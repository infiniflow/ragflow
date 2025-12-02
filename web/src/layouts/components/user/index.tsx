import { useFetchUserInfo } from '@/hooks/use-user-setting-request';
import { Avatar } from 'antd';
import React from 'react';
import { history } from 'umi';

import styles from '../../index.less';

const App: React.FC = () => {
  const { data: userInfo } = useFetchUserInfo();

  const toSetting = () => {
    history.push('/user-setting');
  };

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
