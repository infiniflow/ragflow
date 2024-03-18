import { Button, Card, Flex } from 'antd';

import { useSelectUserInfo } from '@/hooks/userSettingHook';
import styles from './index.less';

const UserSettingTeam = () => {
  const userInfo = useSelectUserInfo();

  return (
    <div className={styles.teamWrapper}>
      <Card className={styles.teamCard}>
        <Flex align="center" justify={'space-between'}>
          <span>{userInfo.nickname} Workspace</span>
          <Button type="primary">Upgrade</Button>
        </Flex>
      </Card>
    </div>
  );
};

export default UserSettingTeam;
