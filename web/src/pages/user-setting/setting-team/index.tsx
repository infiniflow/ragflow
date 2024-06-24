import { Button, Card, Flex } from 'antd';

import { useTranslate } from '@/hooks/commonHooks';
import { useSelectUserInfo } from '@/hooks/userSettingHook';
import styles from './index.less';

const UserSettingTeam = () => {
  const userInfo = useSelectUserInfo();
  const { t } = useTranslate('setting');

  return (
    <div className={styles.teamWrapper}>
      <Card className={styles.teamCard}>
        <Flex align="center" justify={'space-between'}>
          <span>
            {userInfo.nickname} {t('workspace')}
          </span>
          <Button type="primary" disabled>
            {t('upgrade')}
          </Button>
        </Flex>
      </Card>
    </div>
  );
};

export default UserSettingTeam;
