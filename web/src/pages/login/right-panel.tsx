import { ReactComponent as Avatars } from '@/assets/svg/login-avatars.svg';
import SvgIcon from '@/components/svg-icon';
import { Flex, Rate, Space, Typography } from 'antd';
import classNames from 'classnames';

import { useTranslate } from '@/hooks/common-hooks';
import styles from './index.less';

const { Title, Text } = Typography;

const LoginRightPanel = () => {
  const { t } = useTranslate('login');
  return (
    <section className={styles.rightPanel}>
      <SvgIcon name="login-star" width={80}></SvgIcon>
      <Flex vertical gap={40}>
        <Title
          level={1}
          className={classNames(styles.white, styles.loginTitle)}
        >
          {t('title')}
        </Title>
        <Text className={classNames(styles.pink, styles.loginDescription)}>
          {t('description')}
        </Text>
      </Flex>
    </section>
  );
};

export default LoginRightPanel;
