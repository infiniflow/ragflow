import SvgIcon from '@/components/svg-icon';
import { useFetchSystemStatus } from '@/hooks/userSettingHook';
import { ISystemStatus, Minio } from '@/interfaces/database/userSetting';
import { Badge, Card, Flex, Spin, Typography } from 'antd';
import classNames from 'classnames';
import lowerCase from 'lodash/lowerCase';
import upperFirst from 'lodash/upperFirst';
import { useEffect } from 'react';

import { toFixed } from '@/utils/commonUtil';
import styles from './index.less';

const { Text } = Typography;

enum Status {
  'green' = 'success',
  'red' = 'error',
  'yellow' = 'warning',
}

const TitleMap = {
  es: 'Elasticsearch',
  minio: 'MinIO Object Storage',
  redis: 'Redis',
  mysql: 'Mysql',
};

const SystemInfo = () => {
  const {
    systemStatus,
    fetchSystemStatus,
    loading: statusLoading,
  } = useFetchSystemStatus();

  useEffect(() => {
    fetchSystemStatus();
  }, [fetchSystemStatus]);

  return (
    <section className={styles.systemInfo}>
      <Spin spinning={statusLoading}>
        <Flex gap={16} vertical>
          {Object.keys(systemStatus).map((key) => {
            const info = systemStatus[key as keyof ISystemStatus];

            return (
              <Card
                type="inner"
                title={
                  <Flex align="center" gap={10}>
                    <SvgIcon name={key} width={26}></SvgIcon>
                    <span className={styles.title}>
                      {TitleMap[key as keyof typeof TitleMap]}
                    </span>
                    <Badge
                      className={styles.badge}
                      status={Status[info.status as keyof typeof Status]}
                    />
                  </Flex>
                }
                key={key}
              >
                {Object.keys(info)
                  .filter((x) => x !== 'status')
                  .map((x) => {
                    return (
                      <Flex
                        key={x}
                        align="center"
                        gap={16}
                        className={styles.text}
                      >
                        <b>{upperFirst(lowerCase(x))}:</b>
                        <Text
                          className={classNames({
                            [styles.error]: x === 'error',
                          })}
                        >
                          {toFixed(info[x as keyof Minio]) as any}
                          {x === 'elapsed' && ' ms'}
                        </Text>
                      </Flex>
                    );
                  })}
              </Card>
            );
          })}
        </Flex>
      </Spin>
    </section>
  );
};

export default SystemInfo;
