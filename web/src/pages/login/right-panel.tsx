import { ReactComponent as Avatars } from '@/assets/svg/login-avatars.svg';
import SvgIcon from '@/components/svg-icon';
import { Flex, Rate, Space, Typography } from 'antd';
import classNames from 'classnames';

import styles from './index.less';

const { Title, Text } = Typography;

const LoginRightPanel = () => {
  return (
    <section className={styles.rightPanel}>
      <SvgIcon name="login-star" width={80}></SvgIcon>
      <Flex vertical gap={40}>
        <Title
          level={1}
          className={classNames(styles.white, styles.loginTitle)}
        >
          Start building your smart assisstants.
        </Title>
        <Text className={classNames(styles.pink, styles.loginDescription)}>
          Sign up for free to explore top RAG technology. Create knowledge bases
          and AIs to empower your business.
        </Text>
        <Flex align="center" gap={16}>
          <Avatars></Avatars>
          <Flex vertical>
            <Space>
              <Rate disabled defaultValue={5} />
              <span
                className={classNames(styles.white, styles.loginRateNumber)}
              >
                5.0
              </span>
            </Space>
            <span className={classNames(styles.pink, styles.loginRateReviews)}>
              from 500+ reviews
            </span>
          </Flex>
        </Flex>
      </Flex>
    </section>
  );
};

export default LoginRightPanel;
