import { Button, Card, Flex } from 'antd';

import { useTranslate } from '@/hooks/common-hooks';
import { useFetchUserInfo } from '@/hooks/user-setting-hooks';
import styles from './index.less';

const UserSettingTeam = () => {
  const { data: userInfo } = useFetchUserInfo();
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
